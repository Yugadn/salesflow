//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// team is a region with two salespeople and a warehouse worker, each with
// their own login.
type team struct {
	world
	otherSalespersonID int64
	sales, otherSales  string // tokens
	storekeeper        string
}

func newTeam(t *testing.T) team {
	t.Helper()
	w := newWorld(t, 100)
	sfx := time.Now().UnixNano()

	var sp idOnly
	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/salespersons",
		map[string]any{"code": fmt.Sprintf("S2-%d", sfx), "name": "Budi", "region_id": w.regionID}, &sp)

	user := func(email, role string, salesperson int64) string {
		body := map[string]any{"email": email, "name": role, "password": "secret123", "role": role}
		if salesperson > 0 {
			body["salesperson_id"] = salesperson
		}
		mustCall(t, http.StatusCreated, http.MethodPost, authURL+"/users", body, nil)
		return login(t, email, "secret123")
	}
	return team{
		world:              w,
		otherSalespersonID: sp.ID,
		sales:              user(fmt.Sprintf("andi-%d@test.local", sfx), "sales", w.salespersonID),
		otherSales:         user(fmt.Sprintf("budi-%d@test.local", sfx), "sales", sp.ID),
		storekeeper:        user(fmt.Sprintf("wh-%d@test.local", sfx), "warehouse", 0),
	}
}

func (tm team) orderBody() map[string]any {
	return map[string]any{
		"customer_id": tm.customerID,
		"lines":       []map[string]any{{"product_id": tm.productID, "uom": "DUS", "qty": 1}},
	}
}

func TestAuth_EveryServiceRequiresAToken(t *testing.T) {
	for _, url := range []string{masterURL + "/regions", orderURL + "/orders", warehouseURL + "/deliveries", billingURL + "/invoices", authURL + "/users"} {
		status, _, _ := request(t, "", http.MethodGet, url, nil, nil, nil)
		assert.Equal(t, http.StatusUnauthorized, status, url)
	}
	status, _, _ := request(t, "not-a-real-token", http.MethodGet, orderURL+"/orders", nil, nil, nil)
	assert.Equal(t, http.StatusUnauthorized, status)
}

func TestAuth_RolesAreEnforced(t *testing.T) {
	tm := newTeam(t)
	tests := []struct {
		name, token, method, url string
		want                     int
	}{
		{"sales reads master data", tm.sales, "GET", masterURL + "/products", 200},
		{"sales cannot edit master data", tm.sales, "POST", masterURL + "/regions", 403},
		{"sales cannot see invoices", tm.sales, "GET", billingURL + "/invoices", 403},
		{"sales cannot manage users", tm.sales, "GET", authURL + "/users", 403},
		{"warehouse reads deliveries", tm.storekeeper, "GET", warehouseURL + "/deliveries", 200},
		{"warehouse receives stock", tm.storekeeper, "POST", fmt.Sprintf("%s/warehouses/%d/receipts", warehouseURL, tm.warehouseID), 200},
		{"warehouse cannot create orders", tm.storekeeper, "POST", orderURL + "/orders", 403},
		{"warehouse cannot edit products", tm.storekeeper, "PUT", fmt.Sprintf("%s/products/%d", masterURL, tm.productID), 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body any = map[string]any{}
			if tt.name == "warehouse receives stock" {
				body = map[string]any{"product_id": tm.productID, "qty": 1}
			}
			status, apiErr, _ := request(t, tt.token, tt.method, tt.url, body, nil, nil)
			assert.Equal(t, tt.want, status, "%+v", apiErr)
		})
	}
}

func TestAuth_SalesWorkOnlyTheirOwnOrders(t *testing.T) {
	tm := newTeam(t)

	var o order
	status, apiErr, _ := request(t, tm.sales, http.MethodPost, orderURL+"/orders", tm.orderBody(), &o, nil)
	require.Equal(t, http.StatusCreated, status, "%+v", apiErr)

	t.Run("cannot order as another salesperson", func(t *testing.T) {
		body := tm.orderBody()
		body["salesperson_id"] = tm.otherSalespersonID
		status, apiErr, _ := request(t, tm.sales, http.MethodPost, orderURL+"/orders", body, nil, nil)
		assert.Equal(t, http.StatusForbidden, status)
		require.NotNil(t, apiErr)
		assert.Equal(t, "not_your_order", apiErr.Code)
	})

	t.Run("another salesperson cannot see or confirm it", func(t *testing.T) {
		status, _, _ := request(t, tm.otherSales, http.MethodGet, fmt.Sprintf("%s/orders/%d", orderURL, o.ID), nil, nil, nil)
		assert.Equal(t, http.StatusNotFound, status, "404 rather than 403: the order's existence is not revealed")
		status, _, _ = request(t, tm.otherSales, http.MethodPost, fmt.Sprintf("%s/orders/%d/confirm", orderURL, o.ID), nil, nil, nil)
		assert.Equal(t, http.StatusNotFound, status)

		var theirs []order
		request(t, tm.otherSales, http.MethodGet, orderURL+"/orders", nil, &theirs, nil)
		for _, x := range theirs {
			assert.NotEqual(t, o.ID, x.ID)
		}
	})

	t.Run("the owner and an admin can", func(t *testing.T) {
		status, _, _ := request(t, tm.sales, http.MethodGet, fmt.Sprintf("%s/orders/%d", orderURL, o.ID), nil, nil, nil)
		assert.Equal(t, http.StatusOK, status)
		status, _ = call(t, http.MethodGet, fmt.Sprintf("%s/orders/%d", orderURL, o.ID), nil, nil)
		assert.Equal(t, http.StatusOK, status)
	})
}

func TestIdempotency_DoubleSubmitCreatesOneOrder(t *testing.T) {
	tm := newTeam(t)
	key := map[string]string{"Idempotency-Key": fmt.Sprintf("e2e-%d", time.Now().UnixNano())}

	var first, second order
	status, apiErr, _ := request(t, tm.sales, http.MethodPost, orderURL+"/orders", tm.orderBody(), &first, key)
	require.Equal(t, http.StatusCreated, status, "%+v", apiErr)

	status, _, headers := request(t, tm.sales, http.MethodPost, orderURL+"/orders", tm.orderBody(), &second, key)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "true", headers.Get("Idempotent-Replayed"))
	assert.Equal(t, first.ID, second.ID, "the retry returns the same order")

	var mine []order
	request(t, tm.sales, http.MethodGet, orderURL+"/orders", nil, &mine, nil)
	assert.Len(t, mine, 1)

	changed := tm.orderBody()
	changed["lines"] = []map[string]any{{"product_id": tm.productID, "uom": "DUS", "qty": 2}}
	status, apiErr, _ = request(t, tm.sales, http.MethodPost, orderURL+"/orders", changed, nil, key)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	require.NotNil(t, apiErr)
	assert.Equal(t, "idempotency_key_reused", apiErr.Code)
}
