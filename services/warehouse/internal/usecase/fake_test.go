package usecase

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
)

type stockKey struct{ warehouseID, productID int64 }

// fakeStore is an in-memory Store whose stock rules mirror the SQL
// constraints. WithinTx restores a snapshot when fn fails.
type fakeStore struct {
	warehouses map[int64]domain.Warehouse
	stocks     map[stockKey]domain.Stock
	deliveries map[int64]domain.Delivery
	outbox     []events.Envelope
	processed  map[string]bool
	nextID     int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		warehouses: map[int64]domain.Warehouse{},
		stocks:     map[stockKey]domain.Stock{},
		deliveries: map[int64]domain.Delivery{},
		processed:  map[string]bool{},
	}
}

func (f *fakeStore) WithinTx(_ context.Context, fn func(r Repo) error) error {
	wh, st, dl, ob, pr, id := maps.Clone(f.warehouses), maps.Clone(f.stocks), maps.Clone(f.deliveries),
		slices.Clone(f.outbox), maps.Clone(f.processed), f.nextID
	if err := fn(f); err != nil {
		f.warehouses, f.stocks, f.deliveries, f.outbox, f.processed, f.nextID = wh, st, dl, ob, pr, id
		return err
	}
	return nil
}

func (f *fakeStore) CreateWarehouse(_ context.Context, w domain.Warehouse) (domain.Warehouse, error) {
	f.nextID++
	w.ID = f.nextID
	f.warehouses[w.ID] = w
	return w, nil
}

func (f *fakeStore) ListWarehouses(context.Context) ([]domain.Warehouse, error) { return nil, nil }

func (f *fakeStore) GetWarehouse(_ context.Context, id int64) (domain.Warehouse, error) {
	w, ok := f.warehouses[id]
	if !ok {
		return domain.Warehouse{}, apperr.NotFound("warehouse_not_found", "warehouse not found")
	}
	return w, nil
}

func (f *fakeStore) GetWarehouseByRegion(_ context.Context, regionID int64) (domain.Warehouse, error) {
	for _, w := range f.warehouses {
		if w.RegionID == regionID {
			return w, nil
		}
	}
	return domain.Warehouse{}, apperr.NotFound("warehouse_not_found", "warehouse not found")
}

func (f *fakeStore) ReceiveStock(_ context.Context, warehouseID, productID, qty int64) (domain.Stock, error) {
	k := stockKey{warehouseID, productID}
	s := f.stocks[k]
	s.WarehouseID, s.ProductID = warehouseID, productID
	s.OnHand += qty
	f.stocks[k] = s
	return s, nil
}

func (f *fakeStore) ListStocks(context.Context, int64) ([]domain.Stock, error) { return nil, nil }

func (f *fakeStore) LockStocks(_ context.Context, warehouseID int64, productIDs []int64) (map[int64]domain.Stock, error) {
	out := map[int64]domain.Stock{}
	for _, id := range productIDs {
		if s, ok := f.stocks[stockKey{warehouseID, id}]; ok {
			out[id] = s
		}
	}
	return out, nil
}

func (f *fakeStore) Reserve(_ context.Context, warehouseID, productID, qty int64) error {
	k := stockKey{warehouseID, productID}
	s, ok := f.stocks[k]
	if !ok || s.Available() < qty {
		return fmt.Errorf("reserve: not enough available")
	}
	s.Reserved += qty
	f.stocks[k] = s
	return nil
}

func (f *fakeStore) ApplyMovement(_ context.Context, warehouseID int64, m domain.Movement) error {
	k := stockKey{warehouseID, m.ProductID}
	s, ok := f.stocks[k]
	if !ok || s.OnHand < m.Consume || s.Reserved < m.Release {
		return fmt.Errorf("apply movement: invariant violated")
	}
	s.OnHand -= m.Consume
	s.Reserved -= m.Release
	f.stocks[k] = s
	return nil
}

func (f *fakeStore) DeliveryExistsForOrder(_ context.Context, orderID int64) (bool, error) {
	for _, d := range f.deliveries {
		if d.OrderID == orderID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) CreateDelivery(_ context.Context, d *domain.Delivery) error {
	f.nextID++
	d.ID = f.nextID
	d.Number = fmt.Sprintf("DO-%06d", d.ID)
	f.deliveries[d.ID] = cloneDelivery(*d)
	return nil
}

func (f *fakeStore) GetDelivery(_ context.Context, id int64) (domain.Delivery, error) {
	d, ok := f.deliveries[id]
	if !ok {
		return domain.Delivery{}, apperr.NotFound("delivery_not_found", "delivery not found")
	}
	return cloneDelivery(d), nil
}

func (f *fakeStore) GetDeliveryForUpdate(ctx context.Context, id int64) (domain.Delivery, error) {
	return f.GetDelivery(ctx, id)
}

func (f *fakeStore) ListDeliveries(context.Context, domain.DeliveryStatus, int) ([]domain.Delivery, error) {
	return nil, nil
}

func (f *fakeStore) UpdateDelivery(_ context.Context, d domain.Delivery) error {
	f.deliveries[d.ID] = cloneDelivery(d)
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

// cloneDelivery copies the lines slice so the store never aliases a caller's delivery.
func cloneDelivery(d domain.Delivery) domain.Delivery {
	d.Lines = slices.Clone(d.Lines)
	return d
}
