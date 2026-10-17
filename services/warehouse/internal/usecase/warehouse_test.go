package usecase

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
)

const (
	jakarta = int64(1)
	tea     = int64(3) // sold by the carton (24) and by the piece
	coffee  = int64(9)
)

type env struct {
	svc   *Service
	store *fakeStore
	whID  int64
}

func setup(t *testing.T, teaOnHand, coffeeOnHand int64) env {
	t.Helper()
	ctx := context.Background()
	store := newFakeStore()
	svc := NewService(store)
	wh, err := svc.CreateWarehouse(ctx, domain.Warehouse{Code: "WH-JKT", Name: "Gudang Jakarta", RegionID: jakarta})
	require.NoError(t, err)
	if teaOnHand > 0 {
		_, err = svc.ReceiveStock(ctx, wh.ID, tea, teaOnHand)
		require.NoError(t, err)
	}
	if coffeeOnHand > 0 {
		_, err = svc.ReceiveStock(ctx, wh.ID, coffee, coffeeOnHand)
		require.NoError(t, err)
	}
	return env{svc: svc, store: store, whID: wh.ID}
}

func (e env) stock(productID int64) domain.Stock { return e.store.stocks[stockKey{e.whID, productID}] }

// orderConfirmed: 2 cartons + 6 pieces of tea (54 pcs) and 10 coffee.
func orderConfirmed(t *testing.T, orderID, regionID int64) events.Envelope {
	t.Helper()
	ev, err := events.New("order-service", events.OrderConfirmed, events.OrderConfirmedPayload{
		OrderID: orderID, OrderNumber: "SO-000001", CustomerID: 10, CustomerName: "Toko Maju",
		RegionID: regionID, PaymentTermDays: 30,
		Lines: []events.OrderLine{
			{LineNo: 1, ProductID: tea, SKU: "TEA", UoM: "DUS", ConversionFactor: 24, Qty: 2, BaseQty: 48, UnitPrice: decimal.NewFromInt(96000)},
			{LineNo: 2, ProductID: tea, SKU: "TEA", UoM: "PCS", ConversionFactor: 1, Qty: 6, BaseQty: 6, UnitPrice: decimal.NewFromInt(4200)},
			{LineNo: 3, ProductID: coffee, SKU: "KOPI", UoM: "PCS", ConversionFactor: 1, Qty: 10, BaseQty: 10, UnitPrice: decimal.NewFromInt(1500)},
		},
	})
	require.NoError(t, err)
	return ev
}

func lastEvent(t *testing.T, s *fakeStore) events.Envelope {
	t.Helper()
	require.NotEmpty(t, s.outbox)
	return s.outbox[len(s.outbox)-1]
}

func TestOrderConfirmed_ReservesStockAndCreatesDelivery(t *testing.T) {
	e := setup(t, 100, 10)

	require.NoError(t, e.svc.HandleEvent(context.Background(), orderConfirmed(t, 1, jakarta)))

	assert.Equal(t, domain.Stock{WarehouseID: e.whID, ProductID: tea, OnHand: 100, Reserved: 54}, e.stock(tea))
	assert.Equal(t, int64(10), e.stock(coffee).Reserved, "exactly enough coffee is fine")
	require.Len(t, e.store.deliveries, 1)

	ev := lastEvent(t, e.store)
	assert.Equal(t, events.DeliveryCreated, ev.Type)
	var p events.DeliveryCreatedPayload
	require.NoError(t, ev.Decode(&p))
	assert.Equal(t, int64(1), p.OrderID)
	assert.Equal(t, e.whID, p.WarehouseID)
}

func TestOrderConfirmed_ShortageRejectsWholeOrder(t *testing.T) {
	e := setup(t, 100, 9) // one coffee short

	require.NoError(t, e.svc.HandleEvent(context.Background(), orderConfirmed(t, 1, jakarta)),
		"a shortage is a business outcome; the message is acknowledged")

	assert.Zero(t, e.stock(tea).Reserved, "all-or-nothing: tea must not stay reserved")
	assert.Empty(t, e.store.deliveries)

	ev := lastEvent(t, e.store)
	assert.Equal(t, events.StockReservationFailed, ev.Type)
	var p events.StockReservationFailedPayload
	require.NoError(t, ev.Decode(&p))
	assert.Equal(t, "insufficient stock", p.Reason)
	assert.Equal(t, []events.Shortage{{ProductID: coffee, SKU: "KOPI", Requested: 10, Available: 9}}, p.Shortages)
}

func TestOrderConfirmed_NoWarehouseForRegion(t *testing.T) {
	e := setup(t, 100, 10)

	require.NoError(t, e.svc.HandleEvent(context.Background(), orderConfirmed(t, 1, 99)))

	ev := lastEvent(t, e.store)
	assert.Equal(t, events.StockReservationFailed, ev.Type)
	var p events.StockReservationFailedPayload
	require.NoError(t, ev.Decode(&p))
	assert.Contains(t, p.Reason, "no warehouse serves region 99")
}

func TestOrderConfirmed_ReservesOnlyOnce(t *testing.T) {
	ctx := context.Background()

	t.Run("same event redelivered", func(t *testing.T) {
		e := setup(t, 100, 10)
		ev := orderConfirmed(t, 1, jakarta)
		require.NoError(t, e.svc.HandleEvent(ctx, ev))
		require.NoError(t, e.svc.HandleEvent(ctx, ev))
		assert.Equal(t, int64(54), e.stock(tea).Reserved)
		assert.Len(t, e.store.outbox, 1)
	})

	t.Run("same order replayed under a new event id", func(t *testing.T) {
		e := setup(t, 100, 10)
		require.NoError(t, e.svc.HandleEvent(ctx, orderConfirmed(t, 1, jakarta)))
		require.NoError(t, e.svc.HandleEvent(ctx, orderConfirmed(t, 1, jakarta)))
		assert.Equal(t, int64(54), e.stock(tea).Reserved)
		assert.Len(t, e.store.deliveries, 1)
	})
}

func TestOrderConfirmed_MalformedPayloadIsPermanent(t *testing.T) {
	e := setup(t, 0, 0)

	err := e.svc.HandleEvent(context.Background(), events.Envelope{
		ID: "00000000-0000-0000-0000-000000000001", Type: events.OrderConfirmed, Payload: []byte(`{"order_id":"x"}`),
	})

	_, ok := apperr.As(err)
	assert.True(t, ok)
}

func TestShipAndDeliver_MovesStockAndEmitsCompletion(t *testing.T) {
	ctx := context.Background()
	e := setup(t, 100, 10)
	require.NoError(t, e.svc.HandleEvent(ctx, orderConfirmed(t, 1, jakarta)))
	var deliveryID int64
	for id := range e.store.deliveries {
		deliveryID = id
	}

	_, err := e.svc.Ship(ctx, deliveryID)
	require.NoError(t, err)
	// One carton damaged in transit; everything else arrives.
	d, err := e.svc.Deliver(ctx, deliveryID, []domain.DeliveredQty{{LineNo: 1, Qty: 1}})
	require.NoError(t, err)

	assert.Equal(t, domain.DeliveryDelivered, d.Status)
	assert.Equal(t, domain.Stock{WarehouseID: e.whID, ProductID: tea, OnHand: 100 - 30, Reserved: 0}, e.stock(tea),
		"24 + 6 pieces left the shelf; the damaged carton's reservation is released")
	assert.Equal(t, domain.Stock{WarehouseID: e.whID, ProductID: coffee, OnHand: 0, Reserved: 0}, e.stock(coffee))

	ev := lastEvent(t, e.store)
	assert.Equal(t, events.DeliveryCompleted, ev.Type)
	var p events.DeliveryCompletedPayload
	require.NoError(t, ev.Decode(&p))
	assert.Equal(t, 30, p.PaymentTermDays)
	require.Len(t, p.Lines, 3)
	assert.Equal(t, int64(1), p.Lines[0].DeliveredQty)
	assert.Equal(t, int64(2), p.Lines[0].OrderedQty)
	assert.Equal(t, "96000", p.Lines[0].UnitPrice.String())
}

func TestDeliver_InvalidQuantityChangesNothing(t *testing.T) {
	ctx := context.Background()
	e := setup(t, 100, 10)
	require.NoError(t, e.svc.HandleEvent(ctx, orderConfirmed(t, 1, jakarta)))
	var deliveryID int64
	for id := range e.store.deliveries {
		deliveryID = id
	}
	_, err := e.svc.Ship(ctx, deliveryID)
	require.NoError(t, err)
	before := len(e.store.outbox)

	_, err = e.svc.Deliver(ctx, deliveryID, []domain.DeliveredQty{{LineNo: 1, Qty: 5}})

	require.Error(t, err)
	assert.Equal(t, int64(54), e.stock(tea).Reserved)
	assert.Len(t, e.store.outbox, before)
	assert.Equal(t, domain.DeliveryShipped, e.store.deliveries[deliveryID].Status)
}

func TestReceiveStock_Validation(t *testing.T) {
	e := setup(t, 0, 0)
	ctx := context.Background()

	_, err := e.svc.ReceiveStock(ctx, e.whID, tea, 0)
	assert.Error(t, err)

	_, err = e.svc.ReceiveStock(ctx, 999, tea, 10)
	assert.True(t, apperr.IsNotFound(err))
}
