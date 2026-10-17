// Package usecase implements invoicing.
package usecase

import (
	"context"
	"time"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/billing/internal/domain"
)

const source = "billing-service"

// SubscribedEvents are the routing keys billing-service consumes.
var SubscribedEvents = []string{events.DeliveryCompleted}

type Repo interface {
	InvoiceExistsForDelivery(ctx context.Context, deliveryID int64) (bool, error)
	CreateInvoice(ctx context.Context, inv *domain.Invoice) error
	GetInvoice(ctx context.Context, id int64) (domain.Invoice, error)
	ListInvoices(ctx context.Context, customerID int64, limit int) ([]domain.Invoice, error)
	Emit(ctx context.Context, env events.Envelope) error
	MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error)
}

type Store interface {
	Repo
	WithinTx(ctx context.Context, fn func(r Repo) error) error
}

type Service struct {
	store Store
	loc   *time.Location // business time zone for invoice dates
}

func NewService(store Store, loc *time.Location) *Service {
	return &Service{store: store, loc: loc}
}

func (s *Service) Get(ctx context.Context, id int64) (domain.Invoice, error) {
	return s.store.GetInvoice(ctx, id)
}

func (s *Service) List(ctx context.Context, customerID int64, limit int) ([]domain.Invoice, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListInvoices(ctx, customerID, limit)
}

// HandleEvent issues an invoice when a delivery is completed. Two guards
// prevent double-billing: the inbox drops a redelivered event, and the
// per-delivery check drops a replay of the same delivery under a new event ID
// (backed by a unique key on invoices.delivery_id).
func (s *Service) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != events.DeliveryCompleted {
		return nil
	}
	var p events.DeliveryCompletedPayload
	if err := env.Decode(&p); err != nil {
		return apperr.Invalid("bad_payload", "%v", err)
	}
	inv, err := domain.NewInvoice(p, s.loc)
	if err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(r Repo) error {
		first, err := r.MarkProcessed(ctx, env.ID, env.Type)
		if err != nil || !first {
			return err
		}
		exists, err := r.InvoiceExistsForDelivery(ctx, p.DeliveryID)
		if err != nil || exists {
			return err
		}
		if err := r.CreateInvoice(ctx, &inv); err != nil {
			return err
		}
		out, err := events.New(source, events.InvoiceCreated, inv.CreatedPayload())
		if err != nil {
			return err
		}
		return r.Emit(ctx, out)
	})
}
