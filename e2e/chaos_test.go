//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests break the running stack on purpose, so they only run with
// E2E_CHAOS=1. They need the docker CLI and the compose file.
const composeFile = "../deploy/docker-compose.yml"

func requireChaos(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_CHAOS") != "1" {
		t.Skip("set E2E_CHAOS=1 to run tests that stop containers")
	}
}

func compose(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", append([]string{"compose", "-f", composeFile}, args...)...).CombinedOutput()
	require.NoError(t, err, "docker compose %v: %s", args, out)
	return string(out)
}

// deliveredOrder runs an order up to a SHIPPED delivery and returns both IDs.
func shippedOrder(t *testing.T, w world) (orderID, deliveryID int64) {
	t.Helper()
	o := w.placeOrder(t)
	waitForStatus(t, o.ID, "FULFILLING")
	var deliveries []delivery
	mustCall(t, http.StatusOK, http.MethodGet, warehouseURL+"/deliveries?limit=200", nil, &deliveries)
	for _, d := range deliveries {
		if d.OrderID == o.ID {
			deliveryID = d.ID
		}
	}
	require.NotZero(t, deliveryID)
	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/deliveries/%d/ship", warehouseURL, deliveryID), nil, nil)
	return o.ID, deliveryID
}

func invoicesFor(t *testing.T, customerID int64) []invoice {
	t.Helper()
	var invs []invoice
	mustCall(t, http.StatusOK, http.MethodGet, fmt.Sprintf("%s/invoices?customer_id=%d", billingURL, customerID), nil, &invs)
	return invs
}

// TestChaos_InvoiceSurvivesBillingOutage: the delivery is completed while
// billing-service is down. The event waits in billing's durable queue and the
// invoice is issued once the service comes back. Nothing is lost.
func TestChaos_InvoiceSurvivesBillingOutage(t *testing.T) {
	requireChaos(t)
	w := newWorld(t, 100)
	orderID, deliveryID := shippedOrder(t, w)

	compose(t, "stop", "billing")
	t.Cleanup(func() { compose(t, "start", "billing") })

	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/deliveries/%d/deliver", warehouseURL, deliveryID), nil, nil)
	waitForStatus(t, orderID, "DELIVERED")
	time.Sleep(2 * time.Second) // give a hypothetical lost event time to be lost
	assert.Equal(t, "DELIVERED", getOrder(t, orderID).Status, "no invoice while billing is down")

	compose(t, "start", "billing")
	waitForBillingHealthy(t)

	o := waitForStatus(t, orderID, "INVOICED")
	invs := invoicesFor(t, w.customerID)
	require.Len(t, invs, 1)
	assert.Equal(t, o.InvoiceNumber, invs[0].Number)
}

// TestChaos_RedeliveredEventDoesNotDoubleBill republishes a delivery.completed
// event billing has already processed, as RabbitMQ may do after a consumer
// crash. The inbox must drop it.
func TestChaos_RedeliveredEventDoesNotDoubleBill(t *testing.T) {
	requireChaos(t)
	w := newWorld(t, 100)
	orderID, deliveryID := shippedOrder(t, w)
	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/deliveries/%d/deliver", warehouseURL, deliveryID), nil, nil)
	waitForStatus(t, orderID, "INVOICED")
	require.Len(t, invoicesFor(t, w.customerID), 1)

	// Fetch the exact envelope warehouse published, from its outbox.
	envelope := strings.TrimSpace(compose(t, "exec", "-T", "postgres", "psql", "-U", "salesflow", "-d", "warehouse_db", "-tAc",
		fmt.Sprintf(`SELECT envelope FROM outbox WHERE event_type = 'delivery.completed' AND envelope->'payload'->>'delivery_id' = '%d'`, deliveryID)))
	require.NotEmpty(t, envelope)

	publish(t, "delivery.completed", envelope)
	publish(t, "delivery.completed", envelope)
	time.Sleep(2 * time.Second)

	assert.Len(t, invoicesFor(t, w.customerID), 1, "still exactly one invoice")
	assert.Equal(t, "INVOICED", getOrder(t, orderID).Status)
}

// publish sends a raw message through the RabbitMQ management API.
func publish(t *testing.T, routingKey, body string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"properties":       map[string]any{"delivery_mode": 2, "content_type": "application/json"},
		"routing_key":      routingKey,
		"payload":          body,
		"payload_encoding": "string",
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, env("RABBITMQ_API", "http://localhost:15672")+"/api/exchanges/%2F/salesflow.events/publish", bytes.NewReader(raw))
	require.NoError(t, err)
	req.SetBasicAuth("guest", "guest")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Routed bool `json:"routed"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.True(t, out.Routed, "message was not routed to any queue")
}

func waitForBillingHealthy(t *testing.T) {
	t.Helper()
	eventually(t, "billing-service to be healthy", func() bool {
		resp, err := http.Get(billingURL + "/healthz")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
}
