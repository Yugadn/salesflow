package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
)

var jakarta = MustLoadLocation("Asia/Jakarta")

func delivery(deliveredAt time.Time, lines ...events.DeliveredLine) events.DeliveryCompletedPayload {
	return events.DeliveryCompletedPayload{
		DeliveryID: 7, DeliveryNumber: "DO-000007", OrderID: 5, OrderNumber: "SO-000005",
		CustomerID: 10, CustomerName: "Toko Maju", PaymentTermDays: 30,
		DeliveredAt: deliveredAt, Lines: lines,
	}
}

func line(no int, ordered, delivered int64, price string) events.DeliveredLine {
	return events.DeliveredLine{LineNo: no, ProductID: int64(no), SKU: "SKU", UoM: "DUS",
		OrderedQty: ordered, DeliveredQty: delivered, UnitPrice: decimal.RequireFromString(price)}
}

func TestNewInvoice_BillsDeliveredQuantities(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)

	inv, err := NewInvoice(delivery(at,
		line(1, 2, 1, "96000"),   // one carton damaged: bill one
		line(2, 6, 6, "1333.33"), // full
		line(3, 4, 0, "5000"),    // refused entirely: not billed
	), jakarta)

	require.NoError(t, err)
	require.Len(t, inv.Lines, 2)
	assert.Equal(t, "96000", inv.Lines[0].Amount.String())
	assert.Equal(t, "7999.98", inv.Lines[1].Amount.String())
	assert.Equal(t, "103999.98", inv.Total.String())
	assert.Equal(t, 2, inv.Lines[1].LineNo, "keeps the order line number for traceability")
}

func TestNewInvoice_DatesUseBusinessTimeZone(t *testing.T) {
	// 23:30 UTC on the 20th is 06:30 WIB on the 21st.
	at := time.Date(2026, 10, 20, 23, 30, 0, 0, time.UTC)

	inv, err := NewInvoice(delivery(at, line(1, 1, 1, "1000")), jakarta)

	require.NoError(t, err)
	assert.Equal(t, "2026-10-21", inv.IssueDate.String())
	assert.Equal(t, "2026-11-20", inv.DueDate.String(), "issue date + 30 days")
}

func TestNewInvoice_CashCustomerIsDueOnDelivery(t *testing.T) {
	p := delivery(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC), line(1, 1, 1, "1000"))
	p.PaymentTermDays = 0

	inv, err := NewInvoice(p, jakarta)

	require.NoError(t, err)
	assert.Equal(t, inv.IssueDate, inv.DueDate)
}

func TestNewInvoice_Errors(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		p        events.DeliveryCompletedPayload
		wantCode string
	}{
		{"nothing delivered", delivery(at, line(1, 2, 0, "1000")), "nothing_to_invoice"},
		{"delivered more than ordered", delivery(at, line(1, 2, 3, "1000")), "invalid_delivery"},
		{"missing delivery time", delivery(time.Time{}, line(1, 1, 1, "1000")), "invalid_delivery"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewInvoice(tt.p, jakarta)
			ae, ok := apperr.As(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, ae.Code)
		})
	}
}

func TestDate_MarshalsAsCalendarDate(t *testing.T) {
	raw, err := json.Marshal(struct {
		D Date `json:"d"`
	}{DateOf(time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC), time.UTC)})

	require.NoError(t, err)
	assert.JSONEq(t, `{"d":"2026-01-05"}`, string(raw))
}
