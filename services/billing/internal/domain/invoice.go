// Package domain holds invoice rules.
package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
)

// Date is a calendar date with no time of day, serialised as YYYY-MM-DD.
type Date struct{ time.Time }

// DateOf returns the calendar date of t as observed in loc.
func DateOf(t time.Time, loc *time.Location) Date {
	y, m, d := t.In(loc).Date()
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

func (d Date) AddDays(n int) Date           { return Date{d.AddDate(0, 0, n)} }
func (d Date) String() string               { return d.Format(time.DateOnly) }
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

type InvoiceLine struct {
	LineNo      int             `json:"line_no"`
	ProductID   int64           `json:"product_id"`
	SKU         string          `json:"sku"`
	ProductName string          `json:"product_name"`
	UoM         string          `json:"uom"`
	Qty         int64           `json:"qty"`
	UnitPrice   decimal.Decimal `json:"unit_price"`
	Amount      decimal.Decimal `json:"amount"`
}

type Invoice struct {
	ID             int64           `json:"id"`
	Number         string          `json:"number"`
	OrderID        int64           `json:"order_id"`
	OrderNumber    string          `json:"order_number"`
	DeliveryID     int64           `json:"delivery_id"`
	DeliveryNumber string          `json:"delivery_number"`
	CustomerID     int64           `json:"customer_id"`
	CustomerName   string          `json:"customer_name"`
	IssueDate      Date            `json:"issue_date"`
	DueDate        Date            `json:"due_date"`
	Total          decimal.Decimal `json:"total"`
	Lines          []InvoiceLine   `json:"lines"`
	CreatedAt      time.Time       `json:"created_at"`
}

// NewInvoice bills what was actually delivered: each line is charged for its
// delivered quantity at the price fixed when the order was placed. The invoice
// is issued on the delivery date (in loc, the business's time zone) and falls
// due after the customer's payment term.
func NewInvoice(p events.DeliveryCompletedPayload, loc *time.Location) (Invoice, error) {
	if p.DeliveredAt.IsZero() {
		return Invoice{}, apperr.Invalid("invalid_delivery", "delivery %s has no delivery time", p.DeliveryNumber)
	}
	if p.PaymentTermDays < 0 {
		return Invoice{}, apperr.Invalid("invalid_delivery", "negative payment term %d", p.PaymentTermDays)
	}

	issued := DateOf(p.DeliveredAt, loc)
	inv := Invoice{
		OrderID:        p.OrderID,
		OrderNumber:    p.OrderNumber,
		DeliveryID:     p.DeliveryID,
		DeliveryNumber: p.DeliveryNumber,
		CustomerID:     p.CustomerID,
		CustomerName:   p.CustomerName,
		IssueDate:      issued,
		DueDate:        issued.AddDays(p.PaymentTermDays),
		Total:          decimal.Zero,
	}
	for _, l := range p.Lines {
		if l.DeliveredQty < 0 || l.DeliveredQty > l.OrderedQty {
			return Invoice{}, apperr.Invalid("invalid_delivery", "line %d: delivered %d of %d", l.LineNo, l.DeliveredQty, l.OrderedQty)
		}
		if l.DeliveredQty == 0 {
			continue // nothing handed over, nothing to bill
		}
		amount := l.UnitPrice.Mul(decimal.NewFromInt(l.DeliveredQty)).Round(2)
		inv.Lines = append(inv.Lines, InvoiceLine{
			LineNo:      l.LineNo,
			ProductID:   l.ProductID,
			SKU:         l.SKU,
			ProductName: l.ProductName,
			UoM:         l.UoM,
			Qty:         l.DeliveredQty,
			UnitPrice:   l.UnitPrice,
			Amount:      amount,
		})
		inv.Total = inv.Total.Add(amount)
	}
	if len(inv.Lines) == 0 {
		return Invoice{}, apperr.Invalid("nothing_to_invoice", "delivery %s delivered nothing", p.DeliveryNumber)
	}
	return inv, nil
}

// CreatedPayload builds the invoice.created event.
func (inv Invoice) CreatedPayload() events.InvoiceCreatedPayload {
	return events.InvoiceCreatedPayload{
		InvoiceID:     inv.ID,
		InvoiceNumber: inv.Number,
		OrderID:       inv.OrderID,
		DeliveryID:    inv.DeliveryID,
		Total:         inv.Total,
		DueDate:       inv.DueDate.String(),
	}
}

// MustLoadLocation loads a time zone or panics; used once at start-up.
func MustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("load time zone %q: %v", name, err))
	}
	return loc
}
