package usecase

import (
	"context"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

// SubscribedEvents are the routing keys order-service consumes.
var SubscribedEvents = []string{
	events.DeliveryCreated,
	events.StockReservationFailed,
	events.DeliveryCompleted,
	events.InvoiceCreated,
}

// transition is what an inbound event asks of an order.
type transition struct {
	orderID       int64
	to            domain.Status
	rejectReason  string
	invoiceNumber string
}

// HandleEvent applies another service's event to the order it concerns.
// apperr errors mean the event can never apply (bad payload, unknown order,
// illegal transition); any other error is transient and worth retrying.
func (s *Service) HandleEvent(ctx context.Context, env events.Envelope) error {
	var t transition
	switch env.Type {
	case events.DeliveryCreated:
		var p events.DeliveryCreatedPayload
		if err := env.Decode(&p); err != nil {
			return apperr.Invalid("bad_payload", "%v", err)
		}
		t = transition{orderID: p.OrderID, to: domain.StatusFulfilling}

	case events.StockReservationFailed:
		var p events.StockReservationFailedPayload
		if err := env.Decode(&p); err != nil {
			return apperr.Invalid("bad_payload", "%v", err)
		}
		t = transition{orderID: p.OrderID, to: domain.StatusRejected, rejectReason: p.Reason}

	case events.DeliveryCompleted:
		var p events.DeliveryCompletedPayload
		if err := env.Decode(&p); err != nil {
			return apperr.Invalid("bad_payload", "%v", err)
		}
		t = transition{orderID: p.OrderID, to: domain.StatusDelivered}

	case events.InvoiceCreated:
		var p events.InvoiceCreatedPayload
		if err := env.Decode(&p); err != nil {
			return apperr.Invalid("bad_payload", "%v", err)
		}
		t = transition{orderID: p.OrderID, to: domain.StatusInvoiced, invoiceNumber: p.InvoiceNumber}

	default:
		return nil // not ours; acknowledge and move on
	}
	return s.apply(ctx, env, t)
}

// apply runs in one transaction: dedupe the event, lock the order, apply the
// transition. If anything fails the inbox mark rolls back too, so a retried
// delivery is processed again instead of being skipped as a duplicate.
func (s *Service) apply(ctx context.Context, env events.Envelope, t transition) error {
	return s.store.WithinTx(ctx, func(r Repo) error {
		first, err := r.MarkProcessed(ctx, env.ID, env.Type)
		if err != nil || !first {
			return err
		}
		o, err := r.GetOrderForUpdate(ctx, t.orderID)
		if err != nil {
			return err
		}
		changed, err := o.Advance(t.to, t.rejectReason)
		if err != nil {
			return err
		}
		if t.invoiceNumber != "" && o.InvoiceNumber != t.invoiceNumber {
			o.InvoiceNumber = t.invoiceNumber
			changed = true
		}
		if !changed {
			return nil
		}
		return r.UpdateOrder(ctx, o)
	})
}
