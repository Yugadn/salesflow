// Package repository implements the master-data Repository on PostgreSQL
// using sqlc-generated queries.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/services/master/internal/domain"
	"github.com/yugadn/salesflow/services/master/internal/repository/sqlcdb"
)

type Postgres struct {
	pool *pgxpool.Pool
	q    *sqlcdb.Queries
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool, q: sqlcdb.New(pool)}
}

// mapErr turns driver errors into application errors.
func mapErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.NotFound(what+"_not_found", "%s not found", what)
	case db.IsUniqueViolation(err):
		return apperr.Conflict(what+"_exists", "%s with this code already exists", what)
	case db.IsForeignKeyViolation(err):
		return apperr.Invalid("unknown_reference", "%s references a record that does not exist", what)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

func (p *Postgres) CreateRegion(ctx context.Context, r domain.Region) (domain.Region, error) {
	row, err := p.q.CreateRegion(ctx, sqlcdb.CreateRegionParams{Code: r.Code, Name: r.Name})
	if err != nil {
		return domain.Region{}, mapErr(err, "region")
	}
	return domain.Region{ID: row.ID, Code: row.Code, Name: row.Name}, nil
}

func (p *Postgres) ListRegions(ctx context.Context) ([]domain.Region, error) {
	rows, err := p.q.ListRegions(ctx)
	if err != nil {
		return nil, mapErr(err, "region")
	}
	out := make([]domain.Region, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Region{ID: r.ID, Code: r.Code, Name: r.Name})
	}
	return out, nil
}

func toCustomer(id int64, code, name string, regionID int64, term int32) domain.Customer {
	return domain.Customer{ID: id, Code: code, Name: name, RegionID: regionID, PaymentTermDays: int(term)}
}

func (p *Postgres) CreateCustomer(ctx context.Context, c domain.Customer) (domain.Customer, error) {
	row, err := p.q.CreateCustomer(ctx, sqlcdb.CreateCustomerParams{
		Code: c.Code, Name: c.Name, RegionID: c.RegionID, PaymentTermDays: int32(c.PaymentTermDays),
	})
	if err != nil {
		return domain.Customer{}, mapErr(err, "customer")
	}
	return toCustomer(row.ID, row.Code, row.Name, row.RegionID, row.PaymentTermDays), nil
}

func (p *Postgres) GetCustomer(ctx context.Context, id int64) (domain.Customer, error) {
	row, err := p.q.GetCustomer(ctx, id)
	if err != nil {
		return domain.Customer{}, mapErr(err, "customer")
	}
	return toCustomer(row.ID, row.Code, row.Name, row.RegionID, row.PaymentTermDays), nil
}

func (p *Postgres) ListCustomers(ctx context.Context) ([]domain.Customer, error) {
	rows, err := p.q.ListCustomers(ctx)
	if err != nil {
		return nil, mapErr(err, "customer")
	}
	out := make([]domain.Customer, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCustomer(r.ID, r.Code, r.Name, r.RegionID, r.PaymentTermDays))
	}
	return out, nil
}

func (p *Postgres) CreateSalesperson(ctx context.Context, s domain.Salesperson) (domain.Salesperson, error) {
	row, err := p.q.CreateSalesperson(ctx, sqlcdb.CreateSalespersonParams{Code: s.Code, Name: s.Name, RegionID: s.RegionID})
	if err != nil {
		return domain.Salesperson{}, mapErr(err, "salesperson")
	}
	return domain.Salesperson{ID: row.ID, Code: row.Code, Name: row.Name, RegionID: row.RegionID}, nil
}

func (p *Postgres) GetSalesperson(ctx context.Context, id int64) (domain.Salesperson, error) {
	row, err := p.q.GetSalesperson(ctx, id)
	if err != nil {
		return domain.Salesperson{}, mapErr(err, "salesperson")
	}
	return domain.Salesperson{ID: row.ID, Code: row.Code, Name: row.Name, RegionID: row.RegionID}, nil
}

func (p *Postgres) ListSalespersons(ctx context.Context) ([]domain.Salesperson, error) {
	rows, err := p.q.ListSalespersons(ctx)
	if err != nil {
		return nil, mapErr(err, "salesperson")
	}
	out := make([]domain.Salesperson, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Salesperson{ID: r.ID, Code: r.Code, Name: r.Name, RegionID: r.RegionID})
	}
	return out, nil
}

func (p *Postgres) CreateUoM(ctx context.Context, u domain.UoM) (domain.UoM, error) {
	row, err := p.q.CreateUoM(ctx, sqlcdb.CreateUoMParams{Code: u.Code, Name: u.Name})
	if err != nil {
		return domain.UoM{}, mapErr(err, "uom")
	}
	return domain.UoM{Code: row.Code, Name: row.Name}, nil
}

func (p *Postgres) ListUoMs(ctx context.Context) ([]domain.UoM, error) {
	rows, err := p.q.ListUoMs(ctx)
	if err != nil {
		return nil, mapErr(err, "uom")
	}
	out := make([]domain.UoM, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.UoM{Code: r.Code, Name: r.Name})
	}
	return out, nil
}

// CreateProduct inserts the product and its base-UoM conversion atomically.
func (p *Postgres) CreateProduct(ctx context.Context, prod domain.Product) (domain.Product, error) {
	var created domain.Product
	err := db.InTx(ctx, p.pool, func(tx pgx.Tx) error {
		q := p.q.WithTx(tx)
		row, err := q.CreateProduct(ctx, sqlcdb.CreateProductParams{Sku: prod.SKU, Name: prod.Name, BaseUom: prod.BaseUoM})
		if err != nil {
			return mapErr(err, "product")
		}
		if err := q.UpsertProductUoM(ctx, sqlcdb.UpsertProductUoMParams{
			ProductID: row.ID, UomCode: row.BaseUom, Factor: 1,
		}); err != nil {
			return mapErr(err, "product")
		}
		created = domain.Product{
			ID: row.ID, SKU: row.Sku, Name: row.Name, BaseUoM: row.BaseUom,
			UoMs: []domain.ProductUoM{{UoM: row.BaseUom, Factor: 1}},
		}
		return nil
	})
	return created, err
}

func (p *Postgres) GetProduct(ctx context.Context, id int64) (domain.Product, error) {
	row, err := p.q.GetProduct(ctx, id)
	if err != nil {
		return domain.Product{}, mapErr(err, "product")
	}
	prods, err := p.attachUoMs(ctx, []domain.Product{{ID: row.ID, SKU: row.Sku, Name: row.Name, BaseUoM: row.BaseUom}})
	if err != nil {
		return domain.Product{}, err
	}
	return prods[0], nil
}

func (p *Postgres) ListProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := p.q.ListProducts(ctx)
	if err != nil {
		return nil, mapErr(err, "product")
	}
	prods := make([]domain.Product, 0, len(rows))
	for _, r := range rows {
		prods = append(prods, domain.Product{ID: r.ID, SKU: r.Sku, Name: r.Name, BaseUoM: r.BaseUom})
	}
	return p.attachUoMs(ctx, prods)
}

// attachUoMs loads conversions for all products in one query instead of N.
func (p *Postgres) attachUoMs(ctx context.Context, prods []domain.Product) ([]domain.Product, error) {
	if len(prods) == 0 {
		return prods, nil
	}
	ids := make([]int64, len(prods))
	index := make(map[int64]int, len(prods))
	for i, pr := range prods {
		ids[i] = pr.ID
		index[pr.ID] = i
		prods[i].UoMs = []domain.ProductUoM{}
	}
	rows, err := p.q.ListProductUoMs(ctx, ids)
	if err != nil {
		return nil, mapErr(err, "product")
	}
	for _, r := range rows {
		i := index[r.ProductID]
		prods[i].UoMs = append(prods[i].UoMs, domain.ProductUoM{UoM: r.UomCode, Factor: r.Factor})
	}
	return prods, nil
}

func (p *Postgres) UpsertProductUoM(ctx context.Context, productID int64, c domain.ProductUoM) error {
	err := p.q.UpsertProductUoM(ctx, sqlcdb.UpsertProductUoMParams{ProductID: productID, UomCode: c.UoM, Factor: c.Factor})
	return mapErr(err, "uom")
}

func (p *Postgres) GetConversionFactor(ctx context.Context, productID int64, uom string) (int64, error) {
	row, err := p.q.GetProductUoM(ctx, sqlcdb.GetProductUoMParams{ProductID: productID, UomCode: uom})
	if err != nil {
		return 0, mapErr(err, "conversion")
	}
	return row.Factor, nil
}

func (p *Postgres) UpsertPrice(ctx context.Context, pr domain.Price) error {
	err := p.q.UpsertPrice(ctx, sqlcdb.UpsertPriceParams{
		ProductID: pr.ProductID, RegionID: pr.RegionID, UomCode: pr.UoM, Price: pr.Price,
	})
	return mapErr(err, "price")
}

func (p *Postgres) GetPrice(ctx context.Context, productID, regionID int64, uom string) (decimal.Decimal, error) {
	price, err := p.q.GetPrice(ctx, sqlcdb.GetPriceParams{ProductID: productID, RegionID: regionID, UomCode: uom})
	if err != nil {
		return decimal.Decimal{}, mapErr(err, "price")
	}
	return price, nil
}

func (p *Postgres) ListPrices(ctx context.Context, productID int64) ([]domain.Price, error) {
	rows, err := p.q.ListPricesByProduct(ctx, productID)
	if err != nil {
		return nil, mapErr(err, "price")
	}
	out := make([]domain.Price, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Price{ProductID: r.ProductID, RegionID: r.RegionID, UoM: r.UomCode, Price: r.Price})
	}
	return out, nil
}
