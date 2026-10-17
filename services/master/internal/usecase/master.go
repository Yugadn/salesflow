// Package usecase implements master-data operations and the pricing quote
// order-service relies on.
package usecase

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/services/master/internal/domain"
)

// Repository is the persistence port. Implementations return apperr errors:
// NotFound for missing rows, Invalid for unknown references, Conflict for duplicates.
type Repository interface {
	CreateRegion(ctx context.Context, r domain.Region) (domain.Region, error)
	ListRegions(ctx context.Context) ([]domain.Region, error)

	CreateCustomer(ctx context.Context, c domain.Customer) (domain.Customer, error)
	GetCustomer(ctx context.Context, id int64) (domain.Customer, error)
	ListCustomers(ctx context.Context) ([]domain.Customer, error)

	CreateSalesperson(ctx context.Context, s domain.Salesperson) (domain.Salesperson, error)
	GetSalesperson(ctx context.Context, id int64) (domain.Salesperson, error)
	ListSalespersons(ctx context.Context) ([]domain.Salesperson, error)

	CreateUoM(ctx context.Context, u domain.UoM) (domain.UoM, error)
	ListUoMs(ctx context.Context) ([]domain.UoM, error)

	// CreateProduct also registers the base UoM with factor 1.
	CreateProduct(ctx context.Context, p domain.Product) (domain.Product, error)
	GetProduct(ctx context.Context, id int64) (domain.Product, error)
	ListProducts(ctx context.Context) ([]domain.Product, error)
	UpsertProductUoM(ctx context.Context, productID int64, c domain.ProductUoM) error
	GetConversionFactor(ctx context.Context, productID int64, uom string) (int64, error)

	UpsertPrice(ctx context.Context, p domain.Price) error
	GetPrice(ctx context.Context, productID, regionID int64, uom string) (decimal.Decimal, error)
	ListPrices(ctx context.Context, productID int64) ([]domain.Price, error)
}

// Service is the master-data application service.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateRegion(ctx context.Context, r domain.Region) (domain.Region, error) {
	if err := r.Validate(); err != nil {
		return domain.Region{}, err
	}
	return s.repo.CreateRegion(ctx, r)
}

func (s *Service) ListRegions(ctx context.Context) ([]domain.Region, error) {
	return s.repo.ListRegions(ctx)
}

func (s *Service) CreateCustomer(ctx context.Context, c domain.Customer) (domain.Customer, error) {
	if err := c.Validate(); err != nil {
		return domain.Customer{}, err
	}
	return s.repo.CreateCustomer(ctx, c)
}

func (s *Service) GetCustomer(ctx context.Context, id int64) (domain.Customer, error) {
	return s.repo.GetCustomer(ctx, id)
}

func (s *Service) ListCustomers(ctx context.Context) ([]domain.Customer, error) {
	return s.repo.ListCustomers(ctx)
}

func (s *Service) CreateSalesperson(ctx context.Context, sp domain.Salesperson) (domain.Salesperson, error) {
	if err := sp.Validate(); err != nil {
		return domain.Salesperson{}, err
	}
	return s.repo.CreateSalesperson(ctx, sp)
}

func (s *Service) ListSalespersons(ctx context.Context) ([]domain.Salesperson, error) {
	return s.repo.ListSalespersons(ctx)
}

func (s *Service) CreateUoM(ctx context.Context, u domain.UoM) (domain.UoM, error) {
	if err := u.Validate(); err != nil {
		return domain.UoM{}, err
	}
	return s.repo.CreateUoM(ctx, u)
}

func (s *Service) ListUoMs(ctx context.Context) ([]domain.UoM, error) {
	return s.repo.ListUoMs(ctx)
}

func (s *Service) CreateProduct(ctx context.Context, p domain.Product) (domain.Product, error) {
	if err := p.Validate(); err != nil {
		return domain.Product{}, err
	}
	return s.repo.CreateProduct(ctx, p)
}

func (s *Service) GetProduct(ctx context.Context, id int64) (domain.Product, error) {
	return s.repo.GetProduct(ctx, id)
}

func (s *Service) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.repo.ListProducts(ctx)
}

// SetConversion adds or updates how many base units one unit of a UoM holds.
func (s *Service) SetConversion(ctx context.Context, productID int64, c domain.ProductUoM) (domain.Product, error) {
	p, err := s.repo.GetProduct(ctx, productID)
	if err != nil {
		return domain.Product{}, err
	}
	if err := p.ValidateConversion(&c); err != nil {
		return domain.Product{}, err
	}
	if err := s.repo.UpsertProductUoM(ctx, productID, c); err != nil {
		return domain.Product{}, err
	}
	return s.repo.GetProduct(ctx, productID)
}

// SetPrice sets the price of a product in a region for a UoM the product is sold in.
func (s *Service) SetPrice(ctx context.Context, pr domain.Price) (domain.Price, error) {
	if err := pr.Validate(); err != nil {
		return domain.Price{}, err
	}
	if _, err := s.repo.GetProduct(ctx, pr.ProductID); err != nil {
		return domain.Price{}, err
	}
	if _, err := s.repo.GetConversionFactor(ctx, pr.ProductID, pr.UoM); err != nil {
		if apperr.IsNotFound(err) {
			return domain.Price{}, apperr.Invalid("uom_not_configured",
				"product %d is not sold in %s; add a conversion first", pr.ProductID, pr.UoM)
		}
		return domain.Price{}, err
	}
	pr.Price = pr.Price.Round(2)
	if err := s.repo.UpsertPrice(ctx, pr); err != nil {
		return domain.Price{}, err
	}
	return pr, nil
}

func (s *Service) ListPrices(ctx context.Context, productID int64) ([]domain.Price, error) {
	if _, err := s.repo.GetProduct(ctx, productID); err != nil {
		return nil, err
	}
	return s.repo.ListPrices(ctx, productID)
}

// Quote resolves customer, salesperson and region prices for an order draft.
// Region consistency between customer and salesperson is an order rule, so it
// is left to order-service; master only reports both regions.
func (s *Service) Quote(ctx context.Context, req domain.QuoteRequest) (domain.Quote, error) {
	if len(req.Lines) == 0 {
		return domain.Quote{}, apperr.Invalid("empty_quote", "at least one line is required")
	}
	customer, err := s.repo.GetCustomer(ctx, req.CustomerID)
	if err != nil {
		return domain.Quote{}, err
	}
	salesperson, err := s.repo.GetSalesperson(ctx, req.SalespersonID)
	if err != nil {
		return domain.Quote{}, err
	}

	quote := domain.Quote{Customer: customer, Salesperson: salesperson}
	for i, spec := range req.Lines {
		line, err := s.quoteLine(ctx, spec, customer.RegionID)
		if err != nil {
			return domain.Quote{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		quote.Lines = append(quote.Lines, line)
	}
	return quote, nil
}

func (s *Service) quoteLine(ctx context.Context, spec domain.QuoteLineSpec, regionID int64) (domain.QuoteLine, error) {
	uom := domain.NormalizeCode(spec.UoM)
	product, err := s.repo.GetProduct(ctx, spec.ProductID)
	if err != nil {
		return domain.QuoteLine{}, err
	}
	factor, err := s.repo.GetConversionFactor(ctx, product.ID, uom)
	if err != nil {
		if apperr.IsNotFound(err) {
			return domain.QuoteLine{}, apperr.Invalid("uom_not_configured", "product %s is not sold in %s", product.SKU, uom)
		}
		return domain.QuoteLine{}, err
	}
	price, err := s.repo.GetPrice(ctx, product.ID, regionID, uom)
	if err != nil {
		if apperr.IsNotFound(err) {
			return domain.QuoteLine{}, apperr.Invalid("price_not_set",
				"no price for %s per %s in region %d", product.SKU, uom, regionID)
		}
		return domain.QuoteLine{}, err
	}
	return domain.QuoteLine{
		ProductID:        product.ID,
		SKU:              product.SKU,
		ProductName:      product.Name,
		UoM:              uom,
		ConversionFactor: factor,
		UnitPrice:        price,
	}, nil
}
