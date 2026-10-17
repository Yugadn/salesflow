//go:build e2e

// Package e2e drives the running stack (docker compose) over HTTP and checks
// the whole order-to-cash flow across all four services.
//
//	docker compose -f deploy/docker-compose.yml up -d --build
//	go test -tags e2e -v ./e2e
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	masterURL    = env("MASTER_URL", "http://localhost:8081")
	orderURL     = env("ORDER_URL", "http://localhost:8082")
	warehouseURL = env("WAREHOUSE_URL", "http://localhost:8083")
	billingURL   = env("BILLING_URL", "http://localhost:8084")
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---- tiny HTTP client --------------------------------------------------------

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// call sends body as JSON and decodes {"data": ...} into out. It returns the
// status code and, for error responses, the decoded error.
func call(t *testing.T, method, url string, body, out any) (int, *apiError) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "is the stack running? docker compose -f deploy/docker-compose.yml up -d")
	defer func() { _ = resp.Body.Close() }()

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *apiError       `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "%s %s", method, url)
	if out != nil && envelope.Data != nil {
		require.NoError(t, json.Unmarshal(envelope.Data, out))
	}
	return resp.StatusCode, envelope.Error
}

// mustCall fails the test unless the response status is want.
func mustCall(t *testing.T, want int, method, url string, body, out any) {
	t.Helper()
	status, apiErr := call(t, method, url, body, out)
	require.Equal(t, want, status, "%s %s -> %+v", method, url, apiErr)
}

// eventually polls cond until it holds or the timeout expires.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ---- response shapes ---------------------------------------------------------

type idOnly struct {
	ID int64 `json:"id"`
}

type order struct {
	ID            int64  `json:"id"`
	Number        string `json:"number"`
	Status        string `json:"status"`
	Total         string `json:"total"`
	RejectReason  string `json:"reject_reason"`
	InvoiceNumber string `json:"invoice_number"`
	Lines         []struct {
		BaseQty int64 `json:"base_qty"`
	} `json:"lines"`
}

type stock struct {
	ProductID int64 `json:"product_id"`
	OnHand    int64 `json:"on_hand"`
	Reserved  int64 `json:"reserved"`
}

type delivery struct {
	ID      int64  `json:"id"`
	OrderID int64  `json:"order_id"`
	Status  string `json:"status"`
}

type invoice struct {
	ID        int64  `json:"id"`
	Number    string `json:"number"`
	OrderID   int64  `json:"order_id"`
	IssueDate string `json:"issue_date"`
	DueDate   string `json:"due_date"`
	Total     string `json:"total"`
	Lines     []struct {
		LineNo int    `json:"line_no"`
		Qty    int64  `json:"qty"`
		Amount string `json:"amount"`
	} `json:"lines"`
}

// ---- fixture -----------------------------------------------------------------

// world is a fresh region with its own customer, salesperson, product and
// warehouse. Codes carry a per-run suffix so the suite can run repeatedly
// against the same databases.
type world struct {
	regionID, customerID, salespersonID, productID, warehouseID int64
}

func newWorld(t *testing.T, stockPieces int64) world {
	t.Helper()
	sfx := fmt.Sprintf("%d", time.Now().UnixNano())
	var w world
	var r idOnly

	// UoMs are global; a 409 on later runs just means they already exist.
	for _, u := range []map[string]string{{"code": "PCS", "name": "Pieces"}, {"code": "DUS", "name": "Carton"}} {
		status, _ := call(t, http.MethodPost, masterURL+"/uoms", u, nil)
		require.Contains(t, []int{http.StatusCreated, http.StatusConflict}, status)
	}

	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/regions", map[string]any{"code": "R" + sfx, "name": "Region " + sfx}, &r)
	w.regionID = r.ID
	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/customers",
		map[string]any{"code": "C" + sfx, "name": "Toko Maju", "region_id": w.regionID, "payment_term_days": 30}, &r)
	w.customerID = r.ID
	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/salespersons",
		map[string]any{"code": "S" + sfx, "name": "Andi", "region_id": w.regionID}, &r)
	w.salespersonID = r.ID

	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/products",
		map[string]any{"sku": "TEA-" + sfx, "name": "Teh Botol 350ml", "base_uom": "PCS"}, &r)
	w.productID = r.ID
	mustCall(t, http.StatusOK, http.MethodPut, fmt.Sprintf("%s/products/%d/uoms", masterURL, w.productID),
		map[string]any{"uom": "DUS", "factor": 24}, nil)
	for uom, price := range map[string]string{"DUS": "96000", "PCS": "4200"} {
		mustCall(t, http.StatusOK, http.MethodPut, fmt.Sprintf("%s/products/%d/prices", masterURL, w.productID),
			map[string]any{"region_id": w.regionID, "uom": uom, "price": price}, nil)
	}

	mustCall(t, http.StatusCreated, http.MethodPost, warehouseURL+"/warehouses",
		map[string]any{"code": "WH" + sfx, "name": "Gudang " + sfx, "region_id": w.regionID}, &r)
	w.warehouseID = r.ID
	if stockPieces > 0 {
		mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/warehouses/%d/receipts", warehouseURL, w.warehouseID),
			map[string]any{"product_id": w.productID, "qty": stockPieces}, nil)
	}
	return w
}

// placeOrder creates and confirms an order for 2 cartons + 6 pieces (54 pieces).
func (w world) placeOrder(t *testing.T) order {
	t.Helper()
	var o order
	mustCall(t, http.StatusCreated, http.MethodPost, orderURL+"/orders", map[string]any{
		"customer_id":    w.customerID,
		"salesperson_id": w.salespersonID,
		"lines": []map[string]any{
			{"product_id": w.productID, "uom": "DUS", "qty": 2},
			{"product_id": w.productID, "uom": "PCS", "qty": 6},
		},
	}, &o)
	require.Equal(t, "DRAFT", o.Status)
	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/orders/%d/confirm", orderURL, o.ID), nil, &o)
	require.Equal(t, "CONFIRMED", o.Status)
	return o
}

func (w world) stock(t *testing.T) stock {
	t.Helper()
	var all []stock
	mustCall(t, http.StatusOK, http.MethodGet, fmt.Sprintf("%s/warehouses/%d/stocks", warehouseURL, w.warehouseID), nil, &all)
	for _, s := range all {
		if s.ProductID == w.productID {
			return s
		}
	}
	return stock{}
}

func getOrder(t *testing.T, id int64) order {
	t.Helper()
	var o order
	mustCall(t, http.StatusOK, http.MethodGet, fmt.Sprintf("%s/orders/%d", orderURL, id), nil, &o)
	return o
}

func waitForStatus(t *testing.T, id int64, status string) order {
	t.Helper()
	var o order
	eventually(t, "order to become "+status, func() bool {
		o = getOrder(t, id)
		return o.Status == status
	})
	return o
}

// ---- scenarios ---------------------------------------------------------------

func TestOrderToCash(t *testing.T) {
	w := newWorld(t, 100)

	o := w.placeOrder(t)
	assert.Equal(t, "217200", o.Total, "2 x 96000 + 6 x 4200")
	require.Len(t, o.Lines, 2)
	assert.Equal(t, int64(48), o.Lines[0].BaseQty, "2 cartons = 48 pieces")

	// warehouse reserves stock and opens a delivery order
	waitForStatus(t, o.ID, "FULFILLING")
	assert.Equal(t, stock{ProductID: w.productID, OnHand: 100, Reserved: 54}, w.stock(t))

	var deliveries []delivery
	mustCall(t, http.StatusOK, http.MethodGet, warehouseURL+"/deliveries?limit=200", nil, &deliveries)
	var d delivery
	for _, x := range deliveries {
		if x.OrderID == o.ID {
			d = x
		}
	}
	require.NotZero(t, d.ID, "delivery for order %d", o.ID)
	require.Equal(t, "PENDING", d.Status)

	// ship, then deliver with one carton damaged in transit
	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/deliveries/%d/ship", warehouseURL, d.ID), nil, nil)
	mustCall(t, http.StatusOK, http.MethodPost, fmt.Sprintf("%s/deliveries/%d/deliver", warehouseURL, d.ID),
		map[string]any{"lines": []map[string]any{{"line_no": 1, "delivered_qty": 1}}}, nil)

	assert.Equal(t, stock{ProductID: w.productID, OnHand: 70, Reserved: 0}, w.stock(t),
		"30 pieces left the shelf; the damaged carton went back to available")

	// billing invoices what was delivered, and the order learns its invoice number
	o = waitForStatus(t, o.ID, "INVOICED")
	require.NotEmpty(t, o.InvoiceNumber)

	var invoices []invoice
	mustCall(t, http.StatusOK, http.MethodGet, fmt.Sprintf("%s/invoices?customer_id=%d", billingURL, w.customerID), nil, &invoices)
	require.Len(t, invoices, 1)
	inv := invoices[0]
	assert.Equal(t, o.InvoiceNumber, inv.Number)
	assert.Equal(t, "121200", inv.Total, "1 x 96000 + 6 x 4200: only delivered goods are billed")
	require.Len(t, inv.Lines, 2)
	assert.Equal(t, int64(1), inv.Lines[0].Qty)

	issue, err := time.Parse(time.DateOnly, inv.IssueDate)
	require.NoError(t, err)
	assert.Equal(t, issue.AddDate(0, 0, 30).Format(time.DateOnly), inv.DueDate, "30-day payment term")
}

func TestOrderRejectedWhenStockIsShort(t *testing.T) {
	w := newWorld(t, 50) // the order needs 54 pieces

	o := w.placeOrder(t)

	o = waitForStatus(t, o.ID, "REJECTED")
	assert.Equal(t, "insufficient stock", o.RejectReason)
	assert.Equal(t, stock{ProductID: w.productID, OnHand: 50, Reserved: 0}, w.stock(t), "nothing stays reserved")
}

func TestSalespersonCannotSellOutsideTheirRegion(t *testing.T) {
	home := newWorld(t, 100)
	other := newWorld(t, 100)

	status, apiErr := call(t, http.MethodPost, orderURL+"/orders", map[string]any{
		"customer_id":    home.customerID,
		"salesperson_id": other.salespersonID,
		"lines":          []map[string]any{{"product_id": home.productID, "uom": "DUS", "qty": 1}},
	}, nil)

	assert.Equal(t, http.StatusUnprocessableEntity, status)
	require.NotNil(t, apiErr)
	assert.Equal(t, "region_mismatch", apiErr.Code)
}

func TestOrderNeedsAPriceForTheCustomerRegion(t *testing.T) {
	w := newWorld(t, 100)
	var r idOnly
	mustCall(t, http.StatusCreated, http.MethodPost, masterURL+"/products",
		map[string]any{"sku": fmt.Sprintf("NOPRICE-%d", time.Now().UnixNano()), "name": "Unpriced", "base_uom": "PCS"}, &r)

	status, apiErr := call(t, http.MethodPost, orderURL+"/orders", map[string]any{
		"customer_id":    w.customerID,
		"salesperson_id": w.salespersonID,
		"lines":          []map[string]any{{"product_id": r.ID, "uom": "PCS", "qty": 1}},
	}, nil)

	assert.Equal(t, http.StatusUnprocessableEntity, status)
	require.NotNil(t, apiErr)
	assert.Equal(t, "price_not_set", apiErr.Code)
}
