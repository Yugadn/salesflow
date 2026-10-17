// Package repository implements the billing Store on PostgreSQL.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/inbox"
	"github.com/yugadn/salesflow/pkg/outbox"
	"github.com/yugadn/salesflow/services/billing/internal/domain"
	"github.com/yugadn/salesflow/services/billing/internal/repository/sqlcdb"
	"github.com/yugadn/salesflow/services/billing/internal/usecase"
)

type Postgres struct {
	repo
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{repo: repo{q: sqlcdb.New(pool), dbtx: pool}, pool: pool}
}

func (p *Postgres) WithinTx(ctx context.Context, fn func(r usecase.Repo) error) error {
	return db.InTx(ctx, p.pool, func(tx pgx.Tx) error {
		return fn(repo{q: p.q.WithTx(tx), dbtx: tx})
	})
}

type repo struct {
	q    *sqlcdb.Queries
	dbtx db.DBTX
}

func (r repo) InvoiceExistsForDelivery(ctx context.Context, deliveryID int64) (bool, error) {
	exists, err := r.q.InvoiceExistsForDelivery(ctx, deliveryID)
	if err != nil {
		return false, fmt.Errorf("check invoice for delivery %d: %w", deliveryID, err)
	}
	return exists, nil
}

func (r repo) CreateInvoice(ctx context.Context, inv *domain.Invoice) error {
	row, err := r.q.CreateInvoice(ctx, sqlcdb.CreateInvoiceParams{
		OrderID:        inv.OrderID,
		OrderNumber:    inv.OrderNumber,
		DeliveryID:     inv.DeliveryID,
		DeliveryNumber: inv.DeliveryNumber,
		CustomerID:     inv.CustomerID,
		CustomerName:   inv.CustomerName,
		IssueDate:      inv.IssueDate.Time,
		DueDate:        inv.DueDate.Time,
		Total:          inv.Total,
	})
	if err != nil {
		return fmt.Errorf("insert invoice: %w", err)
	}
	inv.ID, inv.Number, inv.CreatedAt = row.ID, row.Number, row.CreatedAt
	for _, l := range inv.Lines {
		if err := r.q.CreateInvoiceLine(ctx, sqlcdb.CreateInvoiceLineParams{
			InvoiceID:   inv.ID,
			LineNo:      int32(l.LineNo),
			ProductID:   l.ProductID,
			Sku:         l.SKU,
			ProductName: l.ProductName,
			Uom:         l.UoM,
			Qty:         l.Qty,
			UnitPrice:   l.UnitPrice,
			Amount:      l.Amount,
		}); err != nil {
			return fmt.Errorf("insert invoice line %d: %w", l.LineNo, err)
		}
	}
	return nil
}

func (r repo) GetInvoice(ctx context.Context, id int64) (domain.Invoice, error) {
	row, err := r.q.GetInvoice(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, apperr.NotFound("invoice_not_found", "invoice not found")
	}
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("get invoice %d: %w", id, err)
	}
	invs, err := r.attachLines(ctx, []domain.Invoice{toInvoice(row)})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invs[0], nil
}

func (r repo) ListInvoices(ctx context.Context, customerID int64, limit int) ([]domain.Invoice, error) {
	var filter *int64
	if customerID > 0 {
		filter = &customerID
	}
	rows, err := r.q.ListInvoices(ctx, sqlcdb.ListInvoicesParams{Limit: int32(limit), CustomerID: filter})
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	out := make([]domain.Invoice, 0, len(rows))
	for _, row := range rows {
		out = append(out, toInvoice(row))
	}
	return r.attachLines(ctx, out)
}

func (r repo) Emit(ctx context.Context, env events.Envelope) error {
	return outbox.Write(ctx, r.dbtx, env)
}

func (r repo) MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error) {
	return inbox.MarkProcessed(ctx, r.dbtx, eventID, eventType)
}

func (r repo) attachLines(ctx context.Context, invs []domain.Invoice) ([]domain.Invoice, error) {
	if len(invs) == 0 {
		return invs, nil
	}
	ids := make([]int64, len(invs))
	index := make(map[int64]int, len(invs))
	for i, inv := range invs {
		ids[i] = inv.ID
		index[inv.ID] = i
		invs[i].Lines = []domain.InvoiceLine{}
	}
	rows, err := r.q.ListInvoiceLines(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list invoice lines: %w", err)
	}
	for _, l := range rows {
		i := index[l.InvoiceID]
		invs[i].Lines = append(invs[i].Lines, domain.InvoiceLine{
			LineNo:      int(l.LineNo),
			ProductID:   l.ProductID,
			SKU:         l.Sku,
			ProductName: l.ProductName,
			UoM:         l.Uom,
			Qty:         l.Qty,
			UnitPrice:   l.UnitPrice,
			Amount:      l.Amount,
		})
	}
	return invs, nil
}

func toInvoice(row sqlcdb.Invoice) domain.Invoice {
	return domain.Invoice{
		ID:             row.ID,
		Number:         row.Number,
		OrderID:        row.OrderID,
		OrderNumber:    row.OrderNumber,
		DeliveryID:     row.DeliveryID,
		DeliveryNumber: row.DeliveryNumber,
		CustomerID:     row.CustomerID,
		CustomerName:   row.CustomerName,
		IssueDate:      domain.Date{Time: row.IssueDate},
		DueDate:        domain.Date{Time: row.DueDate},
		Total:          row.Total,
		CreatedAt:      row.CreatedAt,
	}
}
