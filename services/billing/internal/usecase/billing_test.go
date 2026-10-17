package usecase

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/billing/internal/domain"
)

type fakeStore struct {
	invoices  map[int64]domain.Invoice
	outbox    []events.Envelope
	processed map[string]bool
	nextID    int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{invoices: map[int64]domain.Invoice{}, processed: map[string]bool{}}
}

func (f *fakeStore) WithinTx(_ context.Context, fn func(r Repo) error) error {
	inv, ob, pr, id := maps.Clone(f.invoices), slices.Clone(f.outbox), maps.Clone(f.processed), f.nextID
	if err := fn(f); err != nil {
		f.invoices, f.outbox, f.processed, f.nextID = inv, ob, pr, id
		return err
	}
	return nil
}

func (f *fakeStore) InvoiceExistsForDelivery(_ context.Context, deliveryID int64) (bool, error) {
	for _, inv := range f.invoices {
		if inv.DeliveryID == deliveryID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) CreateInvoice(_ context.Context, inv *domain.Invoice) error {
	f.nextID++
	inv.ID, inv.Number = f.nextID, fmt.Sprintf("INV-%06d", f.nextID)
	f.invoices[inv.ID] = *inv
	return nil
}

func (f *fakeStore) GetInvoice(_ context.Context, id int64) (domain.Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return domain.Invoice{}, apperr.NotFound("invoice_not_found", "invoice not found")
	}
	return inv, nil
}

func (f *fakeStore) ListInvoices(context.Context, int64, int) ([]domain.Invoice, error) {
	return nil, nil
}

func (f *fakeStore) Emit(_ context.Context, env events.Envelope) error {
	f.outbox = append(f.outbox, env)
	return nil
}

func (f *fakeStore) MarkProcessed(_ context.Context, eventID, _ string) (bool, error) {
	if f.processed[eventID] {
		return false, nil
	}
	f.processed[eventID] = true
	return true, nil
}

func newService() (*Service, *fakeStore) {
	store := newFakeStore()
	return NewService(store, domain.MustLoadLocation("Asia/Jakarta")), store
}

func deliveryCompleted(t *testing.T, deliveryID int64) events.Envelope {
	t.Helper()
	env, err := events.New("warehouse-service", events.DeliveryCompleted, events.DeliveryCompletedPayload{
		DeliveryID: deliveryID, DeliveryNumber: "DO-000001", OrderID: 5, OrderNumber: "SO-000005",
		CustomerID: 10, CustomerName: "Toko Maju", PaymentTermDays: 14,
		DeliveredAt: time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC),
		Lines: []events.DeliveredLine{
			{LineNo: 1, ProductID: 3, SKU: "TEA", UoM: "DUS", OrderedQty: 2, DeliveredQty: 2, UnitPrice: decimal.NewFromInt(96000)},
		},
	})
	require.NoError(t, err)
	return env
}

func TestDeliveryCompleted_IssuesInvoiceAndEmitsEvent(t *testing.T) {
	svc, store := newService()

	require.NoError(t, svc.HandleEvent(context.Background(), deliveryCompleted(t, 1)))

	require.Len(t, store.invoices, 1)
	inv := store.invoices[1]
	assert.Equal(t, "INV-000001", inv.Number)
	assert.Equal(t, "192000", inv.Total.String())
	assert.Equal(t, "2026-11-03", inv.DueDate.String())

	require.Len(t, store.outbox, 1)
	var p events.InvoiceCreatedPayload
	require.NoError(t, store.outbox[0].Decode(&p))
	assert.Equal(t, events.InvoiceCreated, store.outbox[0].Type)
	assert.Equal(t, int64(5), p.OrderID)
	assert.Equal(t, "INV-000001", p.InvoiceNumber)
	assert.Equal(t, "2026-11-03", p.DueDate)
}

func TestDeliveryCompleted_NeverBillsTwice(t *testing.T) {
	ctx := context.Background()

	t.Run("same event redelivered", func(t *testing.T) {
		svc, store := newService()
		env := deliveryCompleted(t, 1)
		require.NoError(t, svc.HandleEvent(ctx, env))
		require.NoError(t, svc.HandleEvent(ctx, env))
		assert.Len(t, store.invoices, 1)
		assert.Len(t, store.outbox, 1)
	})

	t.Run("same delivery replayed under a new event id", func(t *testing.T) {
		svc, store := newService()
		require.NoError(t, svc.HandleEvent(ctx, deliveryCompleted(t, 1)))
		require.NoError(t, svc.HandleEvent(ctx, deliveryCompleted(t, 1)))
		assert.Len(t, store.invoices, 1)
	})
}

func TestDeliveryCompleted_InvalidDeliveryIsPermanent(t *testing.T) {
	svc, store := newService()
	env, err := events.New("warehouse-service", events.DeliveryCompleted, events.DeliveryCompletedPayload{
		DeliveryID: 1, DeliveredAt: time.Now(),
		Lines: []events.DeliveredLine{{LineNo: 1, OrderedQty: 2, DeliveredQty: 0, UnitPrice: decimal.NewFromInt(1)}},
	})
	require.NoError(t, err)

	err = svc.HandleEvent(context.Background(), env)

	_, ok := apperr.As(err)
	assert.True(t, ok)
	assert.Empty(t, store.invoices)
	assert.False(t, store.processed[env.ID])
}

func TestHandleEvent_IgnoresOtherEvents(t *testing.T) {
	svc, store := newService()
	env, err := events.New("x", events.OrderConfirmed, map[string]int{})
	require.NoError(t, err)

	require.NoError(t, svc.HandleEvent(context.Background(), env))
	assert.Empty(t, store.invoices)
}
