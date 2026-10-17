package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
)

func TestAggregateNeeds_SumsUnitsAndSortsByProduct(t *testing.T) {
	needs := AggregateNeeds([]events.OrderLine{
		{ProductID: 9, SKU: "KOPI", BaseQty: 10},
		{ProductID: 3, SKU: "TEA", UoM: "DUS", BaseQty: 48}, // 2 cartons
		{ProductID: 3, SKU: "TEA", UoM: "PCS", BaseQty: 6},
	})

	require.Len(t, needs, 2)
	assert.Equal(t, Need{ProductID: 3, SKU: "TEA", BaseQty: 54}, needs[0])
	assert.Equal(t, Need{ProductID: 9, SKU: "KOPI", BaseQty: 10}, needs[1])
}

func TestShortages(t *testing.T) {
	needs := []Need{{ProductID: 1, SKU: "A", BaseQty: 10}, {ProductID: 2, SKU: "B", BaseQty: 5}, {ProductID: 3, SKU: "C", BaseQty: 1}}
	stock := map[int64]Stock{
		1: {ProductID: 1, OnHand: 20, Reserved: 10}, // exactly 10 available: enough
		2: {ProductID: 2, OnHand: 8, Reserved: 4},   // 4 available: short by 1
		// product 3 was never received
	}

	got := Shortages(needs, stock)

	assert.Equal(t, []events.Shortage{
		{ProductID: 2, SKU: "B", Requested: 5, Available: 4},
		{ProductID: 3, SKU: "C", Requested: 1, Available: 0},
	}, got)
}

func shippedDelivery() Delivery {
	d := NewDelivery(1, events.OrderConfirmedPayload{
		OrderID: 5, OrderNumber: "SO-000005", CustomerName: "Toko Maju", PaymentTermDays: 30,
		Lines: []events.OrderLine{
			{LineNo: 1, ProductID: 3, SKU: "TEA", UoM: "DUS", ConversionFactor: 24, Qty: 2, BaseQty: 48, UnitPrice: decimal.NewFromInt(96000)},
			{LineNo: 2, ProductID: 3, SKU: "TEA", UoM: "PCS", ConversionFactor: 1, Qty: 6, BaseQty: 6, UnitPrice: decimal.NewFromInt(4200)},
			{LineNo: 3, ProductID: 9, SKU: "KOPI", UoM: "PCS", ConversionFactor: 1, Qty: 10, BaseQty: 10, UnitPrice: decimal.NewFromInt(1500)},
		},
	})
	d.Number = "DO-000001"
	_ = d.Ship(time.Now())
	return d
}

func TestDeliver_FullDelivery(t *testing.T) {
	d := shippedDelivery()
	now := time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC)

	moves, err := d.Deliver(now, nil)

	require.NoError(t, err)
	assert.Equal(t, DeliveryDelivered, d.Status)
	assert.Equal(t, now, *d.DeliveredAt)
	assert.Equal(t, []Movement{
		{ProductID: 3, Consume: 54, Release: 54},
		{ProductID: 9, Consume: 10, Release: 10},
	}, moves)
}

func TestDeliver_ShortDeliveryReleasesWholeReservation(t *testing.T) {
	d := shippedDelivery()

	// One carton arrived damaged and 4 sachets were refused.
	moves, err := d.Deliver(time.Now(), []DeliveredQty{{LineNo: 1, Qty: 1}, {LineNo: 3, Qty: 6}})

	require.NoError(t, err)
	assert.Equal(t, []Movement{
		{ProductID: 3, Consume: 24 + 6, Release: 48 + 6},
		{ProductID: 9, Consume: 6, Release: 10},
	}, moves)

	p := d.CompletedPayload()
	assert.Equal(t, int64(1), p.Lines[0].DeliveredQty)
	assert.Equal(t, int64(6), p.Lines[1].DeliveredQty, "unlisted line delivered in full")
	assert.Equal(t, int64(6), p.Lines[2].DeliveredQty)
}

func TestDeliver_Errors(t *testing.T) {
	tests := []struct {
		name      string
		overrides []DeliveredQty
		wantCode  string
	}{
		{"more than ordered", []DeliveredQty{{LineNo: 1, Qty: 3}}, "invalid_quantity"},
		{"negative", []DeliveredQty{{LineNo: 1, Qty: -1}}, "invalid_quantity"},
		{"unknown line", []DeliveredQty{{LineNo: 7, Qty: 1}}, "unknown_line"},
		{"nothing delivered", []DeliveredQty{{LineNo: 1, Qty: 0}, {LineNo: 2, Qty: 0}, {LineNo: 3, Qty: 0}}, "nothing_delivered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := shippedDelivery()
			_, err := d.Deliver(time.Now(), tt.overrides)
			ae, ok := apperr.As(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, ae.Code)
			assert.Equal(t, DeliveryShipped, d.Status, "a rejected delivery leaves the order untouched")
			for _, l := range d.Lines {
				assert.Nil(t, l.DeliveredQty, "lines must not be half-updated")
			}
		})
	}
}

func TestDeliveryTransitions(t *testing.T) {
	d := NewDelivery(1, events.OrderConfirmedPayload{OrderID: 1})

	_, err := d.Deliver(time.Now(), nil)
	require.Error(t, err, "cannot deliver before shipping")

	require.NoError(t, d.Ship(time.Now()))
	require.Error(t, d.Ship(time.Now()), "cannot ship twice")
}
