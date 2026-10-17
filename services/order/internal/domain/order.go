// Package domain holds the sales-order aggregate and its rules.
package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
)

// Status is the lifecycle state of a sales order.
//
//	DRAFT ──confirm──▶ CONFIRMED ──stock reserved──▶ FULFILLING ──delivered──▶ DELIVERED ──invoiced──▶ INVOICED
//	  │                    │
//	  └──cancel──▶ CANCELLED └──stock short──▶ REJECTED
type Status string

const (
	StatusDraft      Status = "DRAFT"
	StatusConfirmed  Status = "CONFIRMED"
	StatusFulfilling Status = "FULFILLING"
	StatusDelivered  Status = "DELIVERED"
	StatusInvoiced   Status = "INVOICED"
	StatusRejected   Status = "REJECTED"
	StatusCancelled  Status = "CANCELLED"
)

// progress orders the happy-path statuses so event-driven updates can be
// applied monotonically, whatever order the events arrive in.
var progress = map[Status]int{
	StatusDraft:      0,
	StatusConfirmed:  1,
	StatusFulfilling: 2,
	StatusDelivered:  3,
	StatusInvoiced:   4,
}

// IsTerminalFailure reports whether s ends the order without fulfilment.
func (s Status) IsTerminalFailure() bool { return s == StatusRejected || s == StatusCancelled }

// MaxLineQty bounds a single line so base-unit arithmetic cannot overflow.
const MaxLineQty = 1_000_000

type Line struct {
	LineNo           int             `json:"line_no"`
	ProductID        int64           `json:"product_id"`
	SKU              string          `json:"sku"`
	ProductName      string          `json:"product_name"`
	UoM              string          `json:"uom"`
	ConversionFactor int64           `json:"conversion_factor"`
	Qty              int64           `json:"qty"`
	BaseQty          int64           `json:"base_qty"`
	UnitPrice        decimal.Decimal `json:"unit_price"`
	Subtotal         decimal.Decimal `json:"subtotal"`
}

type Order struct {
	ID              int64           `json:"id"`
	Number          string          `json:"number"`
	Status          Status          `json:"status"`
	CustomerID      int64           `json:"customer_id"`
	CustomerCode    string          `json:"customer_code"`
	CustomerName    string          `json:"customer_name"`
	RegionID        int64           `json:"region_id"`
	PaymentTermDays int             `json:"payment_term_days"`
	SalespersonID   int64           `json:"salesperson_id"`
	SalespersonName string          `json:"salesperson_name"`
	Total           decimal.Decimal `json:"total"`
	RejectReason    string          `json:"reject_reason,omitempty"`
	InvoiceNumber   string          `json:"invoice_number,omitempty"`
	Lines           []Line          `json:"lines"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ConfirmedAt     *time.Time      `json:"confirmed_at,omitempty"`
}

// LineRequest is one line as entered by the salesperson.
type LineRequest struct {
	ProductID int64  `json:"product_id"`
	UoM       string `json:"uom"`
	Qty       int64  `json:"qty"`
}

// Quote is the master-data snapshot an order is priced from.
type Quote struct {
	Customer struct {
		ID              int64  `json:"id"`
		Code            string `json:"code"`
		Name            string `json:"name"`
		RegionID        int64  `json:"region_id"`
		PaymentTermDays int    `json:"payment_term_days"`
	} `json:"customer"`
	Salesperson struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		RegionID int64  `json:"region_id"`
	} `json:"salesperson"`
	Lines []QuoteLine `json:"lines"`
}

type QuoteLine struct {
	ProductID        int64           `json:"product_id"`
	SKU              string          `json:"sku"`
	ProductName      string          `json:"product_name"`
	UoM              string          `json:"uom"`
	ConversionFactor int64           `json:"conversion_factor"`
	UnitPrice        decimal.Decimal `json:"unit_price"`
}

// ValidateLines checks a draft's lines before asking master data to price them.
func ValidateLines(lines []LineRequest) error {
	if len(lines) == 0 {
		return apperr.Invalid("empty_order", "an order needs at least one line")
	}
	seen := make(map[string]bool, len(lines))
	for i, l := range lines {
		if l.ProductID <= 0 {
			return apperr.Invalid("invalid_line", "line %d: product_id is required", i+1)
		}
		if l.Qty <= 0 || l.Qty > MaxLineQty {
			return apperr.Invalid("invalid_line", "line %d: qty must be between 1 and %d", i+1, MaxLineQty)
		}
		key := fmt.Sprintf("%d/%s", l.ProductID, strings.ToUpper(strings.TrimSpace(l.UoM)))
		if seen[key] {
			return apperr.Invalid("duplicate_line", "line %d: product %d in %s appears twice; combine the quantities", i+1, l.ProductID, l.UoM)
		}
		seen[key] = true
	}
	return nil
}

// NewOrder builds a DRAFT order from the requested lines and a price quote.
// A salesperson may only sell to customers in their own region.
func NewOrder(lines []LineRequest, q Quote) (Order, error) {
	if err := ValidateLines(lines); err != nil {
		return Order{}, err
	}
	if len(q.Lines) != len(lines) {
		return Order{}, fmt.Errorf("quote has %d lines, order has %d", len(q.Lines), len(lines))
	}
	if q.Salesperson.RegionID != q.Customer.RegionID {
		return Order{}, apperr.Invalid("region_mismatch",
			"salesperson %s covers region %d but customer %s is in region %d",
			q.Salesperson.Name, q.Salesperson.RegionID, q.Customer.Name, q.Customer.RegionID)
	}

	o := Order{
		Status:          StatusDraft,
		CustomerID:      q.Customer.ID,
		CustomerCode:    q.Customer.Code,
		CustomerName:    q.Customer.Name,
		RegionID:        q.Customer.RegionID,
		PaymentTermDays: q.Customer.PaymentTermDays,
		SalespersonID:   q.Salesperson.ID,
		SalespersonName: q.Salesperson.Name,
		Total:           decimal.Zero,
	}
	for i, req := range lines {
		ql := q.Lines[i]
		if ql.ConversionFactor <= 0 {
			return Order{}, fmt.Errorf("line %d: invalid conversion factor %d", i+1, ql.ConversionFactor)
		}
		subtotal := ql.UnitPrice.Mul(decimal.NewFromInt(req.Qty)).Round(2)
		o.Lines = append(o.Lines, Line{
			LineNo:           i + 1,
			ProductID:        ql.ProductID,
			SKU:              ql.SKU,
			ProductName:      ql.ProductName,
			UoM:              ql.UoM,
			ConversionFactor: ql.ConversionFactor,
			Qty:              req.Qty,
			BaseQty:          req.Qty * ql.ConversionFactor,
			UnitPrice:        ql.UnitPrice,
			Subtotal:         subtotal,
		})
		o.Total = o.Total.Add(subtotal)
	}
	return o, nil
}

// Confirm submits a draft for fulfilment.
func (o *Order) Confirm(now time.Time) error {
	if o.Status != StatusDraft {
		return apperr.Conflict("invalid_transition", "order %s is %s; only DRAFT orders can be confirmed", o.Number, o.Status)
	}
	o.Status = StatusConfirmed
	o.ConfirmedAt = &now
	return nil
}

// Cancel withdraws a draft. Confirmed orders already hold reserved stock, so
// cancelling them needs a compensating flow that is out of scope for the MVP.
func (o *Order) Cancel() error {
	if o.Status != StatusDraft {
		return apperr.Conflict("invalid_transition", "order %s is %s; only DRAFT orders can be cancelled", o.Number, o.Status)
	}
	o.Status = StatusCancelled
	return nil
}

// Advance applies a status reported by another service's event. It is
// idempotent and tolerates reordering: moving to a status the order has
// already reached or passed is a no-op. It reports whether anything changed.
func (o *Order) Advance(to Status, reason string) (bool, error) {
	if to == StatusRejected {
		switch o.Status {
		case StatusRejected:
			return false, nil
		case StatusConfirmed:
			o.Status, o.RejectReason = StatusRejected, reason
			return true, nil
		default:
			return false, apperr.Conflict("invalid_transition", "order %s is %s and cannot be rejected", o.Number, o.Status)
		}
	}

	target, ok := progress[to]
	if !ok {
		return false, fmt.Errorf("unsupported target status %s", to)
	}
	if o.Status.IsTerminalFailure() || o.Status == StatusDraft {
		return false, apperr.Conflict("invalid_transition", "order %s is %s and cannot move to %s", o.Number, o.Status, to)
	}
	if target <= progress[o.Status] {
		return false, nil
	}
	o.Status = to
	return true, nil
}
