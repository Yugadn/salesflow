// Package repository implements the order Store on PostgreSQL.
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
	"github.com/yugadn/salesflow/services/order/internal/domain"
	"github.com/yugadn/salesflow/services/order/internal/repository/sqlcdb"
	"github.com/yugadn/salesflow/services/order/internal/usecase"
)

// Postgres is the pool-backed Store. Its repo methods run outside a
// transaction; WithinTx hands fn a repo bound to a single transaction.
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

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("order_not_found", "order not found")
	}
	return fmt.Errorf("order query: %w", err)
}

func (r repo) CreateOrder(ctx context.Context, o *domain.Order) error {
	row, err := r.q.CreateOrder(ctx, sqlcdb.CreateOrderParams{
		Status:          string(o.Status),
		CustomerID:      o.CustomerID,
		CustomerCode:    o.CustomerCode,
		CustomerName:    o.CustomerName,
		RegionID:        o.RegionID,
		PaymentTermDays: int32(o.PaymentTermDays),
		SalespersonID:   o.SalespersonID,
		SalespersonName: o.SalespersonName,
		Total:           o.Total,
	})
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	o.ID, o.Number, o.CreatedAt, o.UpdatedAt = row.ID, row.Number, row.CreatedAt, row.UpdatedAt

	for _, l := range o.Lines {
		if err := r.q.CreateOrderLine(ctx, sqlcdb.CreateOrderLineParams{
			OrderID:          o.ID,
			LineNo:           int32(l.LineNo),
			ProductID:        l.ProductID,
			Sku:              l.SKU,
			ProductName:      l.ProductName,
			Uom:              l.UoM,
			ConversionFactor: l.ConversionFactor,
			Qty:              l.Qty,
			BaseQty:          l.BaseQty,
			UnitPrice:        l.UnitPrice,
			Subtotal:         l.Subtotal,
		}); err != nil {
			return fmt.Errorf("insert order line %d: %w", l.LineNo, err)
		}
	}
	return nil
}

func (r repo) GetOrder(ctx context.Context, id int64) (domain.Order, error) {
	row, err := r.q.GetOrder(ctx, id)
	if err != nil {
		return domain.Order{}, notFound(err)
	}
	return r.withLines(ctx, toOrder(row))
}

func (r repo) GetOrderForUpdate(ctx context.Context, id int64) (domain.Order, error) {
	row, err := r.q.GetOrderForUpdate(ctx, id)
	if err != nil {
		return domain.Order{}, notFound(err)
	}
	return r.withLines(ctx, toOrder(row))
}

func (r repo) ListOrders(ctx context.Context, status domain.Status, limit int) ([]domain.Order, error) {
	var filter *string
	if status != "" {
		s := string(status)
		filter = &s
	}
	rows, err := r.q.ListOrders(ctx, sqlcdb.ListOrdersParams{Limit: int32(limit), Status: filter})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	orders := make([]domain.Order, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, toOrder(row))
	}
	return r.attachLines(ctx, orders)
}

func (r repo) UpdateOrder(ctx context.Context, o domain.Order) error {
	err := r.q.UpdateOrder(ctx, sqlcdb.UpdateOrderParams{
		ID:            o.ID,
		Status:        string(o.Status),
		RejectReason:  nullable(o.RejectReason),
		InvoiceNumber: nullable(o.InvoiceNumber),
		ConfirmedAt:   o.ConfirmedAt,
	})
	if err != nil {
		return fmt.Errorf("update order %d: %w", o.ID, err)
	}
	return nil
}

func (r repo) Emit(ctx context.Context, env events.Envelope) error {
	return outbox.Write(ctx, r.dbtx, env)
}

func (r repo) MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error) {
	return inbox.MarkProcessed(ctx, r.dbtx, eventID, eventType)
}

func (r repo) withLines(ctx context.Context, o domain.Order) (domain.Order, error) {
	orders, err := r.attachLines(ctx, []domain.Order{o})
	if err != nil {
		return domain.Order{}, err
	}
	return orders[0], nil
}

// attachLines loads lines for all orders in one query.
func (r repo) attachLines(ctx context.Context, orders []domain.Order) ([]domain.Order, error) {
	if len(orders) == 0 {
		return orders, nil
	}
	ids := make([]int64, len(orders))
	index := make(map[int64]int, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
		index[o.ID] = i
		orders[i].Lines = []domain.Line{}
	}
	rows, err := r.q.ListOrderLines(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list order lines: %w", err)
	}
	for _, l := range rows {
		i := index[l.OrderID]
		orders[i].Lines = append(orders[i].Lines, domain.Line{
			LineNo:           int(l.LineNo),
			ProductID:        l.ProductID,
			SKU:              l.Sku,
			ProductName:      l.ProductName,
			UoM:              l.Uom,
			ConversionFactor: l.ConversionFactor,
			Qty:              l.Qty,
			BaseQty:          l.BaseQty,
			UnitPrice:        l.UnitPrice,
			Subtotal:         l.Subtotal,
		})
	}
	return orders, nil
}

func toOrder(row sqlcdb.Order) domain.Order {
	return domain.Order{
		ID:              row.ID,
		Number:          row.Number,
		Status:          domain.Status(row.Status),
		CustomerID:      row.CustomerID,
		CustomerCode:    row.CustomerCode,
		CustomerName:    row.CustomerName,
		RegionID:        row.RegionID,
		PaymentTermDays: int(row.PaymentTermDays),
		SalespersonID:   row.SalespersonID,
		SalespersonName: row.SalespersonName,
		Total:           row.Total,
		RejectReason:    deref(row.RejectReason),
		InvoiceNumber:   deref(row.InvoiceNumber),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		ConfirmedAt:     row.ConfirmedAt,
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
