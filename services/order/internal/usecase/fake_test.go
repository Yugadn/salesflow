package usecase

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

// fakeStore is an in-memory Store. WithinTx snapshots state and restores it
// when fn fails, so tests can observe rollback behaviour.
type fakeStore struct {
	orders    map[int64]domain.Order
	outbox    []events.Envelope
	processed map[string]bool
	nextID    int64

	failUpdate error // injected failure for UpdateOrder
}

func newFakeStore() *fakeStore {
	return &fakeStore{orders: map[int64]domain.Order{}, processed: map[string]bool{}}
}

func (f *fakeStore) WithinTx(_ context.Context, fn func(r Repo) error) error {
	orders, outbox, processed, nextID := maps.Clone(f.orders), slices.Clone(f.outbox), maps.Clone(f.processed), f.nextID
	if err := fn(f); err != nil {
		f.orders, f.outbox, f.processed, f.nextID = orders, outbox, processed, nextID
		return err
	}
	return nil
}

func (f *fakeStore) CreateOrder(_ context.Context, o *domain.Order) error {
	f.nextID++
	o.ID = f.nextID
	o.Number = fmt.Sprintf("SO-%06d", o.ID)
	f.orders[o.ID] = *o
	return nil
}

func (f *fakeStore) GetOrder(_ context.Context, id int64) (domain.Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return domain.Order{}, apperr.NotFound("order_not_found", "order not found")
	}
	return o, nil
}

func (f *fakeStore) GetOrderForUpdate(ctx context.Context, id int64) (domain.Order, error) {
	return f.GetOrder(ctx, id)
}

func (f *fakeStore) ListOrders(context.Context, domain.Status, int) ([]domain.Order, error) {
	return nil, nil
}

func (f *fakeStore) UpdateOrder(_ context.Context, o domain.Order) error {
	if f.failUpdate != nil {
		return f.failUpdate
	}
	f.orders[o.ID] = o
	return nil
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

// fakeMaster returns a canned quote or error.
type fakeMaster struct {
	quote domain.Quote
	err   error
	calls int
}

func (m *fakeMaster) Quote(context.Context, int64, int64, []domain.LineRequest) (domain.Quote, error) {
	m.calls++
	return m.quote, m.err
}
