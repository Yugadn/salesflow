package usecase

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/services/master/internal/domain"
)

// fakeRepo is an in-memory Repository for usecase tests. It mirrors the
// error contract of the Postgres implementation.
type fakeRepo struct {
	regions      map[int64]domain.Region
	customers    map[int64]domain.Customer
	salespersons map[int64]domain.Salesperson
	products     map[int64]domain.Product
	prices       map[string]decimal.Decimal
	nextID       int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		regions:      map[int64]domain.Region{},
		customers:    map[int64]domain.Customer{},
		salespersons: map[int64]domain.Salesperson{},
		products:     map[int64]domain.Product{},
		prices:       map[string]decimal.Decimal{},
	}
}

func (f *fakeRepo) id() int64 { f.nextID++; return f.nextID }

func priceKey(productID, regionID int64, uom string) string {
	return fmt.Sprintf("%d/%d/%s", productID, regionID, uom)
}

func (f *fakeRepo) CreateRegion(_ context.Context, r domain.Region) (domain.Region, error) {
	r.ID = f.id()
	f.regions[r.ID] = r
	return r, nil
}

func (f *fakeRepo) ListRegions(context.Context) ([]domain.Region, error) { return nil, nil }

func (f *fakeRepo) CreateCustomer(_ context.Context, c domain.Customer) (domain.Customer, error) {
	if _, ok := f.regions[c.RegionID]; !ok {
		return domain.Customer{}, apperr.Invalid("unknown_reference", "region missing")
	}
	c.ID = f.id()
	f.customers[c.ID] = c
	return c, nil
}

func (f *fakeRepo) GetCustomer(_ context.Context, id int64) (domain.Customer, error) {
	c, ok := f.customers[id]
	if !ok {
		return domain.Customer{}, apperr.NotFound("customer_not_found", "customer not found")
	}
	return c, nil
}

func (f *fakeRepo) ListCustomers(context.Context) ([]domain.Customer, error) { return nil, nil }

func (f *fakeRepo) CreateSalesperson(_ context.Context, s domain.Salesperson) (domain.Salesperson, error) {
	s.ID = f.id()
	f.salespersons[s.ID] = s
	return s, nil
}

func (f *fakeRepo) GetSalesperson(_ context.Context, id int64) (domain.Salesperson, error) {
	s, ok := f.salespersons[id]
	if !ok {
		return domain.Salesperson{}, apperr.NotFound("salesperson_not_found", "salesperson not found")
	}
	return s, nil
}

func (f *fakeRepo) ListSalespersons(context.Context) ([]domain.Salesperson, error) { return nil, nil }

func (f *fakeRepo) CreateUoM(_ context.Context, u domain.UoM) (domain.UoM, error) { return u, nil }

func (f *fakeRepo) ListUoMs(context.Context) ([]domain.UoM, error) { return nil, nil }

func (f *fakeRepo) CreateProduct(_ context.Context, p domain.Product) (domain.Product, error) {
	p.ID = f.id()
	p.UoMs = []domain.ProductUoM{{UoM: p.BaseUoM, Factor: 1}}
	f.products[p.ID] = p
	return p, nil
}

func (f *fakeRepo) GetProduct(_ context.Context, id int64) (domain.Product, error) {
	p, ok := f.products[id]
	if !ok {
		return domain.Product{}, apperr.NotFound("product_not_found", "product not found")
	}
	return p, nil
}

func (f *fakeRepo) ListProducts(context.Context) ([]domain.Product, error) { return nil, nil }

func (f *fakeRepo) UpsertProductUoM(_ context.Context, productID int64, c domain.ProductUoM) error {
	p := f.products[productID]
	for i, u := range p.UoMs {
		if u.UoM == c.UoM {
			p.UoMs[i] = c
			f.products[productID] = p
			return nil
		}
	}
	p.UoMs = append(p.UoMs, c)
	f.products[productID] = p
	return nil
}

func (f *fakeRepo) GetConversionFactor(_ context.Context, productID int64, uom string) (int64, error) {
	for _, u := range f.products[productID].UoMs {
		if u.UoM == uom {
			return u.Factor, nil
		}
	}
	return 0, apperr.NotFound("conversion_not_found", "conversion not found")
}

func (f *fakeRepo) UpsertPrice(_ context.Context, p domain.Price) error {
	f.prices[priceKey(p.ProductID, p.RegionID, p.UoM)] = p.Price
	return nil
}

func (f *fakeRepo) GetPrice(_ context.Context, productID, regionID int64, uom string) (decimal.Decimal, error) {
	p, ok := f.prices[priceKey(productID, regionID, uom)]
	if !ok {
		return decimal.Decimal{}, apperr.NotFound("price_not_found", "price not found")
	}
	return p, nil
}

func (f *fakeRepo) ListPrices(context.Context, int64) ([]domain.Price, error) { return nil, nil }

func (f *fakeRepo) UpdateRegion(_ context.Context, r domain.Region) (domain.Region, error) {
	if _, ok := f.regions[r.ID]; !ok {
		return domain.Region{}, apperr.NotFound("region_not_found", "region not found")
	}
	f.regions[r.ID] = r
	return r, nil
}

func (f *fakeRepo) UpdateCustomer(_ context.Context, c domain.Customer) (domain.Customer, error) {
	if _, ok := f.customers[c.ID]; !ok {
		return domain.Customer{}, apperr.NotFound("customer_not_found", "customer not found")
	}
	f.customers[c.ID] = c
	return c, nil
}

func (f *fakeRepo) UpdateSalesperson(_ context.Context, s domain.Salesperson) (domain.Salesperson, error) {
	if _, ok := f.salespersons[s.ID]; !ok {
		return domain.Salesperson{}, apperr.NotFound("salesperson_not_found", "salesperson not found")
	}
	f.salespersons[s.ID] = s
	return s, nil
}

func (f *fakeRepo) UpdateProduct(_ context.Context, p domain.Product) (domain.Product, error) {
	cur, ok := f.products[p.ID]
	if !ok {
		return domain.Product{}, apperr.NotFound("product_not_found", "product not found")
	}
	cur.SKU, cur.Name = p.SKU, p.Name
	f.products[p.ID] = cur
	return cur, nil
}
