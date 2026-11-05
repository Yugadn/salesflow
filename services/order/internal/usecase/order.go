// Package usecase implements the sales-order workflow.
package usecase

import (
	"context"
	"time"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

const source = "order-service"

// MasterClient prices an order draft from master data.
type MasterClient interface {
	Quote(ctx context.Context, customerID, salespersonID int64, lines []domain.LineRequest) (domain.Quote, error)
}

// Repo is the persistence port. Within Store.WithinTx all calls share one
// transaction, so state changes, outbox events and inbox marks commit together.
type Repo interface {
	CreateOrder(ctx context.Context, o *domain.Order) error
	GetOrder(ctx context.Context, id int64) (domain.Order, error)
	GetOrderForUpdate(ctx context.Context, id int64) (domain.Order, error)
	ListOrders(ctx context.Context, f ListFilter) ([]domain.Order, error)
	UpdateOrder(ctx context.Context, o domain.Order) error
	// Emit stores an event in the transactional outbox.
	Emit(ctx context.Context, env events.Envelope) error
	// MarkProcessed records an inbound event; false means it was handled before.
	MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error)
}

// ListFilter narrows an order listing; zero values mean "any".
type ListFilter struct {
	Status        domain.Status
	SalespersonID int64
	Limit         int
}

type Store interface {
	Repo
	WithinTx(ctx context.Context, fn func(r Repo) error) error
}

type Service struct {
	store  Store
	master MasterClient
	idem   IdempotencyStore
	now    func() time.Time
}

func NewService(store Store, master MasterClient) *Service {
	return &Service{store: store, master: master, now: func() time.Time { return time.Now().UTC() }}
}

// WithIdempotency enables Idempotency-Key support on order creation.
func (s *Service) WithIdempotency(store IdempotencyStore) *Service {
	s.idem = store
	return s
}

type CreateInput struct {
	CustomerID    int64                `json:"customer_id"`
	SalespersonID int64                `json:"salesperson_id"`
	Lines         []domain.LineRequest `json:"lines"`
}

// Ownership rules. Admins act for anyone. A sales user acts only as the
// salesperson on their token; another salesperson's order is reported as not
// found, so its existence does not leak.

func ownedBy(p auth.Principal, o domain.Order) bool {
	return !p.Is(auth.RoleSales) || o.SalespersonID == p.SalespersonID
}

func notFound() error { return apperr.NotFound("order_not_found", "order not found") }

// Create prices the lines against master data and saves a DRAFT order.
func (s *Service) Create(ctx context.Context, p auth.Principal, in CreateInput) (domain.Order, error) {
	if p.Is(auth.RoleSales) {
		if in.SalespersonID == 0 {
			in.SalespersonID = p.SalespersonID
		}
		if in.SalespersonID != p.SalespersonID {
			return domain.Order{}, apperr.Forbidden("not_your_order", "sales users can only create orders as themselves")
		}
	}
	if in.CustomerID <= 0 || in.SalespersonID <= 0 {
		return domain.Order{}, apperr.Invalid("invalid_order", "customer_id and salesperson_id are required")
	}
	if err := domain.ValidateLines(in.Lines); err != nil {
		return domain.Order{}, err
	}
	q, err := s.master.Quote(ctx, in.CustomerID, in.SalespersonID, in.Lines)
	if err != nil {
		return domain.Order{}, err
	}
	o, err := domain.NewOrder(in.Lines, q)
	if err != nil {
		return domain.Order{}, err
	}
	if err := s.store.WithinTx(ctx, func(r Repo) error { return r.CreateOrder(ctx, &o) }); err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id int64) (domain.Order, error) {
	o, err := s.store.GetOrder(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}
	if !ownedBy(p, o) {
		return domain.Order{}, notFound()
	}
	return o, nil
}

// List returns recent orders; a sales user sees only their own.
func (s *Service) List(ctx context.Context, p auth.Principal, f ListFilter) ([]domain.Order, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if p.Is(auth.RoleSales) {
		f.SalespersonID = p.SalespersonID
	}
	return s.store.ListOrders(ctx, f)
}

// Confirm moves a draft to CONFIRMED and emits order.confirmed in the same
// transaction, so the warehouse hears about every confirmed order exactly once.
func (s *Service) Confirm(ctx context.Context, p auth.Principal, id int64) (domain.Order, error) {
	var out domain.Order
	err := s.store.WithinTx(ctx, func(r Repo) error {
		o, err := r.GetOrderForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !ownedBy(p, o) {
			return notFound()
		}
		if err := o.Confirm(s.now()); err != nil {
			return err
		}
		if err := r.UpdateOrder(ctx, o); err != nil {
			return err
		}
		env, err := events.New(source, events.OrderConfirmed, confirmedPayload(o))
		if err != nil {
			return err
		}
		if err := r.Emit(ctx, env); err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}

// Cancel withdraws a DRAFT order.
func (s *Service) Cancel(ctx context.Context, p auth.Principal, id int64) (domain.Order, error) {
	var out domain.Order
	err := s.store.WithinTx(ctx, func(r Repo) error {
		o, err := r.GetOrderForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !ownedBy(p, o) {
			return notFound()
		}
		if err := o.Cancel(); err != nil {
			return err
		}
		out = o
		return r.UpdateOrder(ctx, o)
	})
	return out, err
}

func confirmedPayload(o domain.Order) events.OrderConfirmedPayload {
	p := events.OrderConfirmedPayload{
		OrderID:         o.ID,
		OrderNumber:     o.Number,
		CustomerID:      o.CustomerID,
		CustomerName:    o.CustomerName,
		RegionID:        o.RegionID,
		PaymentTermDays: o.PaymentTermDays,
		Total:           o.Total,
	}
	for _, l := range o.Lines {
		p.Lines = append(p.Lines, events.OrderLine{
			LineNo:           l.LineNo,
			ProductID:        l.ProductID,
			SKU:              l.SKU,
			ProductName:      l.ProductName,
			UoM:              l.UoM,
			ConversionFactor: l.ConversionFactor,
			Qty:              l.Qty,
			BaseQty:          l.BaseQty,
			UnitPrice:        l.UnitPrice,
			Subtotal:         l.Subtotal,
		})
	}
	return p
}
