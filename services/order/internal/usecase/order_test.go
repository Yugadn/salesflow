package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

func jakartaQuote() domain.Quote {
	var q domain.Quote
	q.Customer.ID, q.Customer.Code, q.Customer.Name, q.Customer.RegionID, q.Customer.PaymentTermDays = 10, "C1", "Toko Maju", 1, 30
	q.Salesperson.ID, q.Salesperson.Name, q.Salesperson.RegionID = 20, "Andi", 1
	q.Lines = []domain.QuoteLine{{
		ProductID: 1, SKU: "TEA-350", ProductName: "Teh Botol", UoM: "DUS",
		ConversionFactor: 24, UnitPrice: decimal.RequireFromString("96000"),
	}}
	return q
}

func newService() (*Service, *fakeStore, *fakeMaster) {
	store, master := newFakeStore(), &fakeMaster{quote: jakartaQuote()}
	return NewService(store, master), store, master
}

func draftInput() CreateInput {
	return CreateInput{CustomerID: 10, SalespersonID: 20, Lines: []domain.LineRequest{{ProductID: 1, UoM: "DUS", Qty: 2}}}
}

func TestCreate_SavesPricedDraft(t *testing.T) {
	svc, store, _ := newService()

	o, err := svc.Create(context.Background(), draftInput())

	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, o.Status)
	assert.Equal(t, "SO-000001", o.Number)
	assert.Equal(t, "192000", o.Total.String())
	assert.Contains(t, store.orders, o.ID)
	assert.Empty(t, store.outbox, "drafts publish nothing")
}

func TestCreate_ValidatesBeforeCallingMaster(t *testing.T) {
	svc, _, master := newService()
	in := draftInput()
	in.Lines[0].Qty = 0

	_, err := svc.Create(context.Background(), in)

	require.Error(t, err)
	assert.Zero(t, master.calls, "invalid input must not cost a network call")
}

func TestCreate_PropagatesMasterValidationError(t *testing.T) {
	svc, store, master := newService()
	master.err = apperr.Invalid("price_not_set", "no price")

	_, err := svc.Create(context.Background(), draftInput())

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "price_not_set", ae.Code)
	assert.Empty(t, store.orders)
}

func TestConfirm_EmitsOrderConfirmed(t *testing.T) {
	svc, store, _ := newService()
	ctx := context.Background()
	o, err := svc.Create(ctx, draftInput())
	require.NoError(t, err)

	confirmed, err := svc.Confirm(ctx, o.ID)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusConfirmed, confirmed.Status)
	require.Len(t, store.outbox, 1)
	env := store.outbox[0]
	assert.Equal(t, events.OrderConfirmed, env.Type)

	var p events.OrderConfirmedPayload
	require.NoError(t, env.Decode(&p))
	assert.Equal(t, o.ID, p.OrderID)
	assert.Equal(t, int64(1), p.RegionID)
	assert.Equal(t, 30, p.PaymentTermDays)
	require.Len(t, p.Lines, 1)
	assert.Equal(t, int64(48), p.Lines[0].BaseQty)
	assert.Equal(t, "96000", p.Lines[0].UnitPrice.String())
}

func TestConfirm_TwiceIsRejectedAndEmitsOnce(t *testing.T) {
	svc, store, _ := newService()
	ctx := context.Background()
	o, _ := svc.Create(ctx, draftInput())
	_, err := svc.Confirm(ctx, o.ID)
	require.NoError(t, err)

	_, err = svc.Confirm(ctx, o.ID)

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, apperr.KindConflict, ae.Kind)
	assert.Len(t, store.outbox, 1)
}

func TestConfirm_UnknownOrder(t *testing.T) {
	svc, _, _ := newService()

	_, err := svc.Confirm(context.Background(), 404)

	assert.True(t, apperr.IsNotFound(err))
}

// confirmedOrder returns the ID of an order that has been confirmed.
func confirmedOrder(t *testing.T, svc *Service) int64 {
	t.Helper()
	ctx := context.Background()
	o, err := svc.Create(ctx, draftInput())
	require.NoError(t, err)
	_, err = svc.Confirm(ctx, o.ID)
	require.NoError(t, err)
	return o.ID
}

func event(t *testing.T, eventType string, payload any) events.Envelope {
	t.Helper()
	env, err := events.New("test", eventType, payload)
	require.NoError(t, err)
	return env
}

func TestHandleEvent_FullLifecycle(t *testing.T) {
	svc, store, _ := newService()
	ctx := context.Background()
	id := confirmedOrder(t, svc)

	steps := []struct {
		env  events.Envelope
		want domain.Status
	}{
		{event(t, events.DeliveryCreated, events.DeliveryCreatedPayload{OrderID: id, DeliveryID: 7}), domain.StatusFulfilling},
		{event(t, events.DeliveryCompleted, events.DeliveryCompletedPayload{OrderID: id, DeliveryID: 7}), domain.StatusDelivered},
		{event(t, events.InvoiceCreated, events.InvoiceCreatedPayload{OrderID: id, InvoiceNumber: "INV-000001"}), domain.StatusInvoiced},
	}
	for _, s := range steps {
		require.NoError(t, svc.HandleEvent(ctx, s.env))
		assert.Equal(t, s.want, store.orders[id].Status)
	}
	assert.Equal(t, "INV-000001", store.orders[id].InvoiceNumber)
}

func TestHandleEvent_DuplicateDeliveryIsIgnored(t *testing.T) {
	svc, store, _ := newService()
	ctx := context.Background()
	id := confirmedOrder(t, svc)
	env := event(t, events.StockReservationFailed, events.StockReservationFailedPayload{OrderID: id, Reason: "insufficient stock"})

	require.NoError(t, svc.HandleEvent(ctx, env))
	require.NoError(t, svc.HandleEvent(ctx, env), "redelivery of the same event must succeed quietly")

	assert.Equal(t, domain.StatusRejected, store.orders[id].Status)
	assert.Equal(t, "insufficient stock", store.orders[id].RejectReason)
}

func TestHandleEvent_FailureRollsBackInboxMark(t *testing.T) {
	svc, store, _ := newService()
	ctx := context.Background()
	id := confirmedOrder(t, svc)
	env := event(t, events.DeliveryCreated, events.DeliveryCreatedPayload{OrderID: id})

	store.failUpdate = errors.New("database connection lost")
	err := svc.HandleEvent(ctx, env)
	require.Error(t, err)
	_, isApp := apperr.As(err)
	assert.False(t, isApp, "infrastructure errors must stay retryable")
	assert.False(t, store.processed[env.ID], "a failed attempt must not consume the event")

	store.failUpdate = nil
	require.NoError(t, svc.HandleEvent(ctx, env), "the retry applies the event")
	assert.Equal(t, domain.StatusFulfilling, store.orders[id].Status)
}

func TestHandleEvent_PermanentFailures(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()
	draft, err := svc.Create(ctx, draftInput())
	require.NoError(t, err)

	tests := []struct {
		name string
		env  events.Envelope
	}{
		{"unknown order", event(t, events.DeliveryCreated, events.DeliveryCreatedPayload{OrderID: 999})},
		{"illegal transition", event(t, events.DeliveryCreated, events.DeliveryCreatedPayload{OrderID: draft.ID})},
		{"malformed payload", events.Envelope{ID: "00000000-0000-0000-0000-000000000001", Type: events.DeliveryCreated, Payload: []byte(`{"order_id":"x"}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := apperr.As(svc.HandleEvent(ctx, tt.env))
			assert.True(t, ok, "should be an application error so it is dead-lettered, not retried")
		})
	}
}

func TestHandleEvent_IgnoresUnrelatedEvents(t *testing.T) {
	svc, _, _ := newService()

	err := svc.HandleEvent(context.Background(), event(t, "something.else", map[string]int{}))

	assert.NoError(t, err)
}
