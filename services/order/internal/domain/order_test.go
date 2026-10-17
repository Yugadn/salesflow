package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
)

func quote(customerRegion, salespersonRegion int64, lines ...QuoteLine) Quote {
	var q Quote
	q.Customer.ID, q.Customer.Name, q.Customer.RegionID, q.Customer.PaymentTermDays = 10, "Toko Maju", customerRegion, 30
	q.Salesperson.ID, q.Salesperson.Name, q.Salesperson.RegionID = 20, "Andi", salespersonRegion
	q.Lines = lines
	return q
}

func teaPerCarton() QuoteLine {
	return QuoteLine{ProductID: 1, SKU: "TEA-350", ProductName: "Teh Botol", UoM: "DUS", ConversionFactor: 24, UnitPrice: decimal.RequireFromString("96000")}
}

func coffeePerPiece() QuoteLine {
	return QuoteLine{ProductID: 2, SKU: "KOPI-1", ProductName: "Kopi Sachet", UoM: "PCS", ConversionFactor: 1, UnitPrice: decimal.RequireFromString("1333.33")}
}

func TestNewOrder_PricesAndConvertsUnits(t *testing.T) {
	o, err := NewOrder(
		[]LineRequest{{ProductID: 1, UoM: "DUS", Qty: 3}, {ProductID: 2, UoM: "PCS", Qty: 3}},
		quote(1, 1, teaPerCarton(), coffeePerPiece()),
	)

	require.NoError(t, err)
	assert.Equal(t, StatusDraft, o.Status)
	require.Len(t, o.Lines, 2)

	assert.Equal(t, int64(72), o.Lines[0].BaseQty, "3 cartons x 24 pieces")
	assert.Equal(t, "288000", o.Lines[0].Subtotal.String())

	// 1333.33 x 3 = 3999.99 exactly; float64 would print 3999.9900000000002.
	assert.Equal(t, "3999.99", o.Lines[1].Subtotal.String())
	assert.Equal(t, "291999.99", o.Total.String())
	assert.Equal(t, 2, o.Lines[1].LineNo)
	assert.Equal(t, int64(1), o.RegionID)
	assert.Equal(t, 30, o.PaymentTermDays)
}

func TestNewOrder_RejectsCrossRegionSale(t *testing.T) {
	_, err := NewOrder([]LineRequest{{ProductID: 1, UoM: "DUS", Qty: 1}}, quote(1, 2, teaPerCarton()))

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "region_mismatch", ae.Code)
}

func TestValidateLines(t *testing.T) {
	tests := []struct {
		name     string
		lines    []LineRequest
		wantCode string
	}{
		{"no lines", nil, "empty_order"},
		{"zero qty", []LineRequest{{ProductID: 1, UoM: "DUS", Qty: 0}}, "invalid_line"},
		{"negative qty", []LineRequest{{ProductID: 1, UoM: "DUS", Qty: -5}}, "invalid_line"},
		{"qty over limit", []LineRequest{{ProductID: 1, UoM: "DUS", Qty: MaxLineQty + 1}}, "invalid_line"},
		{"missing product", []LineRequest{{UoM: "DUS", Qty: 1}}, "invalid_line"},
		{"same product and uom twice", []LineRequest{{ProductID: 1, UoM: "DUS", Qty: 1}, {ProductID: 1, UoM: "DUS", Qty: 2}}, "duplicate_line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ae, ok := apperr.As(ValidateLines(tt.lines))
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, ae.Code)
		})
	}

	t.Run("same product in different uoms is fine", func(t *testing.T) {
		require.NoError(t, ValidateLines([]LineRequest{{ProductID: 1, UoM: "DUS", Qty: 1}, {ProductID: 1, UoM: "PCS", Qty: 6}}))
	})
}

func TestConfirmAndCancel(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

	t.Run("draft can be confirmed once", func(t *testing.T) {
		o := Order{Status: StatusDraft}
		require.NoError(t, o.Confirm(now))
		assert.Equal(t, StatusConfirmed, o.Status)
		assert.Equal(t, now, *o.ConfirmedAt)

		err := o.Confirm(now)
		ae, ok := apperr.As(err)
		require.True(t, ok)
		assert.Equal(t, apperr.KindConflict, ae.Kind)
	})

	t.Run("only drafts can be cancelled", func(t *testing.T) {
		draft := Order{Status: StatusDraft}
		require.NoError(t, draft.Cancel())
		assert.Equal(t, StatusCancelled, draft.Status)

		confirmed := Order{Status: StatusConfirmed}
		require.Error(t, confirmed.Cancel())
	})
}

func TestAdvance(t *testing.T) {
	tests := []struct {
		name        string
		from, to    Status
		wantStatus  Status
		wantChanged bool
		wantErr     bool
	}{
		{"stock reserved", StatusConfirmed, StatusFulfilling, StatusFulfilling, true, false},
		{"delivered", StatusFulfilling, StatusDelivered, StatusDelivered, true, false},
		{"invoiced", StatusDelivered, StatusInvoiced, StatusInvoiced, true, false},
		{"invoice event overtakes delivery event", StatusFulfilling, StatusInvoiced, StatusInvoiced, true, false},
		{"late delivery event after invoice is ignored", StatusInvoiced, StatusDelivered, StatusInvoiced, false, false},
		{"duplicate event is a no-op", StatusDelivered, StatusDelivered, StatusDelivered, false, false},
		{"rejected when stock is short", StatusConfirmed, StatusRejected, StatusRejected, true, false},
		{"duplicate rejection is a no-op", StatusRejected, StatusRejected, StatusRejected, false, false},
		{"cannot reject a fulfilling order", StatusFulfilling, StatusRejected, StatusFulfilling, false, true},
		{"rejected order cannot be fulfilled", StatusRejected, StatusFulfilling, StatusRejected, false, true},
		{"cancelled order cannot be fulfilled", StatusCancelled, StatusFulfilling, StatusCancelled, false, true},
		{"draft cannot jump ahead", StatusDraft, StatusFulfilling, StatusDraft, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := Order{Status: tt.from}
			changed, err := o.Advance(tt.to, "insufficient stock")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantChanged, changed)
			assert.Equal(t, tt.wantStatus, o.Status)
		})
	}
}

func TestAdvance_RecordsRejectReason(t *testing.T) {
	o := Order{Status: StatusConfirmed}
	_, err := o.Advance(StatusRejected, "insufficient stock for TEA-350")
	require.NoError(t, err)
	assert.Equal(t, "insufficient stock for TEA-350", o.RejectReason)
}
