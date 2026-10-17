// Package usecase implements stock management and delivery orders.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
)

const source = "warehouse-service"

// Repo is the persistence port; inside Store.WithinTx all calls share one transaction.
type Repo interface {
	CreateWarehouse(ctx context.Context, w domain.Warehouse) (domain.Warehouse, error)
	ListWarehouses(ctx context.Context) ([]domain.Warehouse, error)
	GetWarehouse(ctx context.Context, id int64) (domain.Warehouse, error)
	GetWarehouseByRegion(ctx context.Context, regionID int64) (domain.Warehouse, error)

	ReceiveStock(ctx context.Context, warehouseID, productID, qty int64) (domain.Stock, error)
	ListStocks(ctx context.Context, warehouseID int64) ([]domain.Stock, error)
	// LockStocks locks the given products' rows, in product-ID order.
	LockStocks(ctx context.Context, warehouseID int64, productIDs []int64) (map[int64]domain.Stock, error)
	// Reserve moves qty base units from available to reserved; it fails rather
	// than let reserved exceed on_hand.
	Reserve(ctx context.Context, warehouseID, productID, qty int64) error
	ApplyMovement(ctx context.Context, warehouseID int64, m domain.Movement) error

	DeliveryExistsForOrder(ctx context.Context, orderID int64) (bool, error)
	CreateDelivery(ctx context.Context, d *domain.Delivery) error
	GetDelivery(ctx context.Context, id int64) (domain.Delivery, error)
	GetDeliveryForUpdate(ctx context.Context, id int64) (domain.Delivery, error)
	ListDeliveries(ctx context.Context, status domain.DeliveryStatus, limit int) ([]domain.Delivery, error)
	// UpdateDelivery persists status, timestamps and delivered quantities.
	UpdateDelivery(ctx context.Context, d domain.Delivery) error

	Emit(ctx context.Context, env events.Envelope) error
	MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error)
}

type Store interface {
	Repo
	WithinTx(ctx context.Context, fn func(r Repo) error) error
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) CreateWarehouse(ctx context.Context, w domain.Warehouse) (domain.Warehouse, error) {
	if err := w.Validate(); err != nil {
		return domain.Warehouse{}, err
	}
	return s.store.CreateWarehouse(ctx, w)
}

func (s *Service) ListWarehouses(ctx context.Context) ([]domain.Warehouse, error) {
	return s.store.ListWarehouses(ctx)
}

// ReceiveStock books an inbound receipt of qty base units.
func (s *Service) ReceiveStock(ctx context.Context, warehouseID, productID, qty int64) (domain.Stock, error) {
	if productID <= 0 || qty <= 0 {
		return domain.Stock{}, apperr.Invalid("invalid_receipt", "product_id and a positive qty are required")
	}
	if _, err := s.store.GetWarehouse(ctx, warehouseID); err != nil {
		return domain.Stock{}, err
	}
	return s.store.ReceiveStock(ctx, warehouseID, productID, qty)
}

func (s *Service) ListStocks(ctx context.Context, warehouseID int64) ([]domain.Stock, error) {
	if _, err := s.store.GetWarehouse(ctx, warehouseID); err != nil {
		return nil, err
	}
	return s.store.ListStocks(ctx, warehouseID)
}

func (s *Service) GetDelivery(ctx context.Context, id int64) (domain.Delivery, error) {
	return s.store.GetDelivery(ctx, id)
}

func (s *Service) ListDeliveries(ctx context.Context, status domain.DeliveryStatus, limit int) ([]domain.Delivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListDeliveries(ctx, status, limit)
}

// Ship marks a delivery as dispatched from the warehouse.
func (s *Service) Ship(ctx context.Context, id int64) (domain.Delivery, error) {
	var out domain.Delivery
	err := s.store.WithinTx(ctx, func(r Repo) error {
		d, err := r.GetDeliveryForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := d.Ship(s.now()); err != nil {
			return err
		}
		out = d
		return r.UpdateDelivery(ctx, d)
	})
	return out, err
}

// Deliver records proof of delivery: it takes delivered goods out of stock,
// releases the reservation and emits delivery.completed for billing, all in
// one transaction.
func (s *Service) Deliver(ctx context.Context, id int64, overrides []domain.DeliveredQty) (domain.Delivery, error) {
	var out domain.Delivery
	err := s.store.WithinTx(ctx, func(r Repo) error {
		d, err := r.GetDeliveryForUpdate(ctx, id)
		if err != nil {
			return err
		}
		moves, err := d.Deliver(s.now(), overrides)
		if err != nil {
			return err
		}
		for _, m := range moves {
			if err := r.ApplyMovement(ctx, d.WarehouseID, m); err != nil {
				return err
			}
		}
		if err := r.UpdateDelivery(ctx, d); err != nil {
			return err
		}
		env, err := events.New(source, events.DeliveryCompleted, d.CompletedPayload())
		if err != nil {
			return err
		}
		out = d
		return r.Emit(ctx, env)
	})
	return out, err
}

// SubscribedEvents are the routing keys warehouse-service consumes.
var SubscribedEvents = []string{events.OrderConfirmed}

// HandleEvent reacts to confirmed orders by reserving stock.
func (s *Service) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != events.OrderConfirmed {
		return nil
	}
	var p events.OrderConfirmedPayload
	if err := env.Decode(&p); err != nil {
		return apperr.Invalid("bad_payload", "%v", err)
	}
	return s.store.WithinTx(ctx, func(r Repo) error {
		first, err := r.MarkProcessed(ctx, env.ID, env.Type)
		if err != nil || !first {
			return err
		}
		return s.reserve(ctx, r, p)
	})
}

// reserve tries to reserve the whole order in the warehouse serving its
// region. It is all-or-nothing: either every product is reserved and a
// delivery order is created, or nothing changes and the order is rejected.
// Stock shortage is a business outcome, not an error, so it is reported as
// an event and the message is acknowledged.
func (s *Service) reserve(ctx context.Context, r Repo, p events.OrderConfirmedPayload) error {
	// The order ID is unique per delivery; a second order.confirmed for the
	// same order (e.g. a replay with a new event ID) must not reserve twice.
	exists, err := r.DeliveryExistsForOrder(ctx, p.OrderID)
	if err != nil || exists {
		return err
	}

	wh, err := r.GetWarehouseByRegion(ctx, p.RegionID)
	if apperr.IsNotFound(err) {
		return s.reject(ctx, r, p.OrderID, fmt.Sprintf("no warehouse serves region %d", p.RegionID), nil)
	}
	if err != nil {
		return err
	}

	needs := domain.AggregateNeeds(p.Lines)
	ids := make([]int64, len(needs))
	for i, n := range needs {
		ids[i] = n.ProductID
	}
	stock, err := r.LockStocks(ctx, wh.ID, ids)
	if err != nil {
		return err
	}
	if shortages := domain.Shortages(needs, stock); len(shortages) > 0 {
		return s.reject(ctx, r, p.OrderID, "insufficient stock", shortages)
	}

	for _, n := range needs {
		if err := r.Reserve(ctx, wh.ID, n.ProductID, n.BaseQty); err != nil {
			return err
		}
	}
	d := domain.NewDelivery(wh.ID, p)
	if err := r.CreateDelivery(ctx, &d); err != nil {
		return err
	}
	env, err := events.New(source, events.DeliveryCreated, events.DeliveryCreatedPayload{
		DeliveryID: d.ID, DeliveryNumber: d.Number, OrderID: p.OrderID, WarehouseID: wh.ID,
	})
	if err != nil {
		return err
	}
	return r.Emit(ctx, env)
}

func (s *Service) reject(ctx context.Context, r Repo, orderID int64, reason string, shortages []events.Shortage) error {
	env, err := events.New(source, events.StockReservationFailed, events.StockReservationFailedPayload{
		OrderID: orderID, Reason: reason, Shortages: shortages,
	})
	if err != nil {
		return err
	}
	return r.Emit(ctx, env)
}
