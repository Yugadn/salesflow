// Package domain holds master-data entities and their validation rules.
package domain

import (
	"strings"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
)

type Region struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type Customer struct {
	ID              int64  `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	RegionID        int64  `json:"region_id"`
	PaymentTermDays int    `json:"payment_term_days"`
}

type Salesperson struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	RegionID int64  `json:"region_id"`
}

type UoM struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ProductUoM states how many base units one unit of UoM contains.
type ProductUoM struct {
	UoM    string `json:"uom"`
	Factor int64  `json:"factor"`
}

type Product struct {
	ID      int64        `json:"id"`
	SKU     string       `json:"sku"`
	Name    string       `json:"name"`
	BaseUoM string       `json:"base_uom"`
	UoMs    []ProductUoM `json:"uoms"`
}

type Price struct {
	ProductID int64           `json:"product_id"`
	RegionID  int64           `json:"region_id"`
	UoM       string          `json:"uom"`
	Price     decimal.Decimal `json:"price"`
}

// NormalizeCode uppercases and trims a business code so "jkt " and "JKT" match.
func NormalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func (r *Region) Validate() error {
	r.Code, r.Name = NormalizeCode(r.Code), strings.TrimSpace(r.Name)
	if r.Code == "" || r.Name == "" {
		return apperr.Invalid("invalid_region", "code and name are required")
	}
	return nil
}

func (c *Customer) Validate() error {
	c.Code, c.Name = NormalizeCode(c.Code), strings.TrimSpace(c.Name)
	switch {
	case c.Code == "" || c.Name == "":
		return apperr.Invalid("invalid_customer", "code and name are required")
	case c.RegionID <= 0:
		return apperr.Invalid("invalid_customer", "region_id is required")
	case c.PaymentTermDays < 0 || c.PaymentTermDays > 365:
		return apperr.Invalid("invalid_customer", "payment_term_days must be between 0 and 365")
	}
	return nil
}

func (s *Salesperson) Validate() error {
	s.Code, s.Name = NormalizeCode(s.Code), strings.TrimSpace(s.Name)
	if s.Code == "" || s.Name == "" {
		return apperr.Invalid("invalid_salesperson", "code and name are required")
	}
	if s.RegionID <= 0 {
		return apperr.Invalid("invalid_salesperson", "region_id is required")
	}
	return nil
}

func (u *UoM) Validate() error {
	u.Code, u.Name = NormalizeCode(u.Code), strings.TrimSpace(u.Name)
	if u.Code == "" || u.Name == "" {
		return apperr.Invalid("invalid_uom", "code and name are required")
	}
	return nil
}

func (p *Product) Validate() error {
	p.SKU, p.Name, p.BaseUoM = NormalizeCode(p.SKU), strings.TrimSpace(p.Name), NormalizeCode(p.BaseUoM)
	if p.SKU == "" || p.Name == "" || p.BaseUoM == "" {
		return apperr.Invalid("invalid_product", "sku, name and base_uom are required")
	}
	return nil
}

// ValidateConversion checks a UoM conversion for product p. The base UoM is
// the unit stock is counted in, so its factor is fixed at 1.
func (p Product) ValidateConversion(c *ProductUoM) error {
	c.UoM = NormalizeCode(c.UoM)
	if c.UoM == "" {
		return apperr.Invalid("invalid_conversion", "uom is required")
	}
	if c.Factor <= 0 {
		return apperr.Invalid("invalid_conversion", "factor must be greater than zero")
	}
	if c.UoM == p.BaseUoM && c.Factor != 1 {
		return apperr.Invalid("invalid_conversion", "base uom %s must have factor 1", p.BaseUoM)
	}
	return nil
}

func (pr *Price) Validate() error {
	pr.UoM = NormalizeCode(pr.UoM)
	switch {
	case pr.ProductID <= 0 || pr.RegionID <= 0 || pr.UoM == "":
		return apperr.Invalid("invalid_price", "product_id, region_id and uom are required")
	case pr.Price.IsNegative():
		return apperr.Invalid("invalid_price", "price must not be negative")
	case !pr.Price.Equal(pr.Price.Round(2)):
		return apperr.Invalid("invalid_price", "price must have at most 2 decimal places")
	}
	return nil
}
