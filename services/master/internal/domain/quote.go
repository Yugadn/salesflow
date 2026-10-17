package domain

import "github.com/shopspring/decimal"

// QuoteRequest asks for everything order-service needs to price an order.
type QuoteRequest struct {
	CustomerID    int64           `json:"customer_id"`
	SalespersonID int64           `json:"salesperson_id"`
	Lines         []QuoteLineSpec `json:"lines"`
}

type QuoteLineSpec struct {
	ProductID int64  `json:"product_id"`
	UoM       string `json:"uom"`
}

// Quote is a snapshot of master data, priced for the customer's region.
type Quote struct {
	Customer    Customer    `json:"customer"`
	Salesperson Salesperson `json:"salesperson"`
	Lines       []QuoteLine `json:"lines"`
}

type QuoteLine struct {
	ProductID        int64           `json:"product_id"`
	SKU              string          `json:"sku"`
	ProductName      string          `json:"product_name"`
	UoM              string          `json:"uom"`
	ConversionFactor int64           `json:"conversion_factor"`
	UnitPrice        decimal.Decimal `json:"unit_price"`
}
