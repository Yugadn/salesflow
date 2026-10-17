// Package repository implements the warehouse Store on PostgreSQL.
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
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
	"github.com/yugadn/salesflow/services/warehouse/internal/repository/sqlcdb"
	"github.com/yugadn/salesflow/services/warehouse/internal/usecase"
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

func mapErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.NotFound(what+"_not_found", "%s not found", what)
	case db.IsUniqueViolation(err):
		return apperr.Conflict(what+"_exists", "%s already exists", what)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

func toWarehouse(id int64, code, name string, regionID int64) domain.Warehouse {
	return domain.Warehouse{ID: id, Code: code, Name: name, RegionID: regionID}
}

func (r repo) CreateWarehouse(ctx context.Context, w domain.Warehouse) (domain.Warehouse, error) {
	row, err := r.q.CreateWarehouse(ctx, sqlcdb.CreateWarehouseParams{Code: w.Code, Name: w.Name, RegionID: w.RegionID})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return domain.Warehouse{}, apperr.Conflict("warehouse_exists", "a warehouse with this code, or for this region, already exists")
		}
		return domain.Warehouse{}, mapErr(err, "warehouse")
	}
	return toWarehouse(row.ID, row.Code, row.Name, row.RegionID), nil
}

func (r repo) ListWarehouses(ctx context.Context) ([]domain.Warehouse, error) {
	rows, err := r.q.ListWarehouses(ctx)
	if err != nil {
		return nil, mapErr(err, "warehouse")
	}
	out := make([]domain.Warehouse, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWarehouse(w.ID, w.Code, w.Name, w.RegionID))
	}
	return out, nil
}

func (r repo) GetWarehouse(ctx context.Context, id int64) (domain.Warehouse, error) {
	w, err := r.q.GetWarehouse(ctx, id)
	if err != nil {
		return domain.Warehouse{}, mapErr(err, "warehouse")
	}
	return toWarehouse(w.ID, w.Code, w.Name, w.RegionID), nil
}

func (r repo) GetWarehouseByRegion(ctx context.Context, regionID int64) (domain.Warehouse, error) {
	w, err := r.q.GetWarehouseByRegion(ctx, regionID)
	if err != nil {
		return domain.Warehouse{}, mapErr(err, "warehouse")
	}
	return toWarehouse(w.ID, w.Code, w.Name, w.RegionID), nil
}

func toStock(warehouseID, productID, onHand, reserved int64) domain.Stock {
	return domain.Stock{WarehouseID: warehouseID, ProductID: productID, OnHand: onHand, Reserved: reserved}
}

func (r repo) ReceiveStock(ctx context.Context, warehouseID, productID, qty int64) (domain.Stock, error) {
	s, err := r.q.ReceiveStock(ctx, sqlcdb.ReceiveStockParams{WarehouseID: warehouseID, ProductID: productID, Qty: qty})
	if err != nil {
		return domain.Stock{}, mapErr(err, "stock")
	}
	return toStock(s.WarehouseID, s.ProductID, s.OnHand, s.Reserved), nil
}

func (r repo) ListStocks(ctx context.Context, warehouseID int64) ([]domain.Stock, error) {
	rows, err := r.q.ListStocks(ctx, warehouseID)
	if err != nil {
		return nil, mapErr(err, "stock")
	}
	out := make([]domain.Stock, 0, len(rows))
	for _, s := range rows {
		out = append(out, toStock(s.WarehouseID, s.ProductID, s.OnHand, s.Reserved))
	}
	return out, nil
}

func (r repo) LockStocks(ctx context.Context, warehouseID int64, productIDs []int64) (map[int64]domain.Stock, error) {
	rows, err := r.q.LockStocks(ctx, sqlcdb.LockStocksParams{WarehouseID: warehouseID, ProductIds: productIDs})
	if err != nil {
		return nil, mapErr(err, "stock")
	}
	out := make(map[int64]domain.Stock, len(rows))
	for _, s := range rows {
		out[s.ProductID] = toStock(s.WarehouseID, s.ProductID, s.OnHand, s.Reserved)
	}
	return out, nil
}

func (r repo) Reserve(ctx context.Context, warehouseID, productID, qty int64) error {
	n, err := r.q.ReserveStock(ctx, sqlcdb.ReserveStockParams{WarehouseID: warehouseID, ProductID: productID, Qty: qty})
	if err != nil {
		return mapErr(err, "stock")
	}
	if n != 1 {
		// Rows are locked before this call, so this means a bug, not a race.
		return fmt.Errorf("reserve %d of product %d in warehouse %d: not enough available", qty, productID, warehouseID)
	}
	return nil
}

func (r repo) ApplyMovement(ctx context.Context, warehouseID int64, m domain.Movement) error {
	n, err := r.q.ApplyDeliveryMovement(ctx, sqlcdb.ApplyDeliveryMovementParams{
		WarehouseID: warehouseID, ProductID: m.ProductID, Consume: m.Consume, Release: m.Release,
	})
	if err != nil {
		return mapErr(err, "stock")
	}
	if n != 1 {
		return fmt.Errorf("apply delivery movement for product %d: stock row missing or reservation too small", m.ProductID)
	}
	return nil
}

func (r repo) DeliveryExistsForOrder(ctx context.Context, orderID int64) (bool, error) {
	exists, err := r.q.DeliveryExistsForOrder(ctx, orderID)
	if err != nil {
		return false, mapErr(err, "delivery")
	}
	return exists, nil
}

func (r repo) CreateDelivery(ctx context.Context, d *domain.Delivery) error {
	row, err := r.q.CreateDelivery(ctx, sqlcdb.CreateDeliveryParams{
		OrderID:         d.OrderID,
		OrderNumber:     d.OrderNumber,
		WarehouseID:     d.WarehouseID,
		CustomerID:      d.CustomerID,
		CustomerName:    d.CustomerName,
		PaymentTermDays: int32(d.PaymentTermDays),
		Status:          string(d.Status),
	})
	if err != nil {
		return mapErr(err, "delivery")
	}
	d.ID, d.Number, d.CreatedAt = row.ID, row.Number, row.CreatedAt
	for _, l := range d.Lines {
		if err := r.q.CreateDeliveryLine(ctx, sqlcdb.CreateDeliveryLineParams{
			DeliveryID:       d.ID,
			LineNo:           int32(l.LineNo),
			ProductID:        l.ProductID,
			Sku:              l.SKU,
			ProductName:      l.ProductName,
			Uom:              l.UoM,
			ConversionFactor: l.ConversionFactor,
			OrderedQty:       l.OrderedQty,
			UnitPrice:        l.UnitPrice,
		}); err != nil {
			return fmt.Errorf("insert delivery line %d: %w", l.LineNo, err)
		}
	}
	return nil
}

func (r repo) GetDelivery(ctx context.Context, id int64) (domain.Delivery, error) {
	row, err := r.q.GetDelivery(ctx, id)
	if err != nil {
		return domain.Delivery{}, mapErr(err, "delivery")
	}
	return r.withLines(ctx, toDelivery(row))
}

func (r repo) GetDeliveryForUpdate(ctx context.Context, id int64) (domain.Delivery, error) {
	row, err := r.q.GetDeliveryForUpdate(ctx, id)
	if err != nil {
		return domain.Delivery{}, mapErr(err, "delivery")
	}
	return r.withLines(ctx, toDelivery(row))
}

func (r repo) ListDeliveries(ctx context.Context, f domain.DeliveryFilter) ([]domain.Delivery, error) {
	params := sqlcdb.ListDeliveriesParams{Limit: int32(f.Limit)}
	if f.Status != "" {
		s := string(f.Status)
		params.Status = &s
	}
	if f.OrderID > 0 {
		params.OrderID = &f.OrderID
	}
	rows, err := r.q.ListDeliveries(ctx, params)
	if err != nil {
		return nil, mapErr(err, "delivery")
	}
	out := make([]domain.Delivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDelivery(row))
	}
	return r.attachLines(ctx, out)
}

func (r repo) UpdateDelivery(ctx context.Context, d domain.Delivery) error {
	if err := r.q.UpdateDelivery(ctx, sqlcdb.UpdateDeliveryParams{
		ID: d.ID, Status: string(d.Status), ShippedAt: d.ShippedAt, DeliveredAt: d.DeliveredAt,
	}); err != nil {
		return fmt.Errorf("update delivery %d: %w", d.ID, err)
	}
	for _, l := range d.Lines {
		if l.DeliveredQty == nil {
			continue
		}
		if err := r.q.SetDeliveredQty(ctx, sqlcdb.SetDeliveredQtyParams{
			DeliveryID: d.ID, LineNo: int32(l.LineNo), DeliveredQty: l.DeliveredQty,
		}); err != nil {
			return fmt.Errorf("update delivery %d line %d: %w", d.ID, l.LineNo, err)
		}
	}
	return nil
}

func (r repo) Emit(ctx context.Context, env events.Envelope) error {
	return outbox.Write(ctx, r.dbtx, env)
}

func (r repo) MarkProcessed(ctx context.Context, eventID, eventType string) (bool, error) {
	return inbox.MarkProcessed(ctx, r.dbtx, eventID, eventType)
}

func (r repo) withLines(ctx context.Context, d domain.Delivery) (domain.Delivery, error) {
	ds, err := r.attachLines(ctx, []domain.Delivery{d})
	if err != nil {
		return domain.Delivery{}, err
	}
	return ds[0], nil
}

func (r repo) attachLines(ctx context.Context, ds []domain.Delivery) ([]domain.Delivery, error) {
	if len(ds) == 0 {
		return ds, nil
	}
	ids := make([]int64, len(ds))
	index := make(map[int64]int, len(ds))
	for i, d := range ds {
		ids[i] = d.ID
		index[d.ID] = i
		ds[i].Lines = []domain.DeliveryLine{}
	}
	rows, err := r.q.ListDeliveryLines(ctx, ids)
	if err != nil {
		return nil, mapErr(err, "delivery")
	}
	for _, l := range rows {
		i := index[l.DeliveryID]
		ds[i].Lines = append(ds[i].Lines, domain.DeliveryLine{
			LineNo:           int(l.LineNo),
			ProductID:        l.ProductID,
			SKU:              l.Sku,
			ProductName:      l.ProductName,
			UoM:              l.Uom,
			ConversionFactor: l.ConversionFactor,
			OrderedQty:       l.OrderedQty,
			DeliveredQty:     l.DeliveredQty,
			UnitPrice:        l.UnitPrice,
		})
	}
	return ds, nil
}

func toDelivery(row sqlcdb.Delivery) domain.Delivery {
	return domain.Delivery{
		ID:              row.ID,
		Number:          row.Number,
		OrderID:         row.OrderID,
		OrderNumber:     row.OrderNumber,
		WarehouseID:     row.WarehouseID,
		CustomerID:      row.CustomerID,
		CustomerName:    row.CustomerName,
		PaymentTermDays: int(row.PaymentTermDays),
		Status:          domain.DeliveryStatus(row.Status),
		CreatedAt:       row.CreatedAt,
		ShippedAt:       row.ShippedAt,
		DeliveredAt:     row.DeliveredAt,
	}
}

func (r repo) UpdateWarehouse(ctx context.Context, w domain.Warehouse) (domain.Warehouse, error) {
	row, err := r.q.UpdateWarehouse(ctx, sqlcdb.UpdateWarehouseParams{ID: w.ID, Code: w.Code, Name: w.Name})
	if err != nil {
		return domain.Warehouse{}, mapErr(err, "warehouse")
	}
	return toWarehouse(row.ID, row.Code, row.Name, row.RegionID), nil
}
