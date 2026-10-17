// Package domain holds warehouse stock and delivery-order rules.
//
// Stock is counted in each product's base unit. For every warehouse/product:
//
//	on_hand   physical units on the shelf
//	reserved  units promised to confirmed orders but not yet delivered
//	available on_hand - reserved, what a new order may reserve
package domain

import (
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
)

type Warehouse struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	RegionID int64  `json:"region_id"`
}

func (w *Warehouse) Validate() error {
	w.Code, w.Name = strings.ToUpper(strings.TrimSpace(w.Code)), strings.TrimSpace(w.Name)
	if w.Code == "" || w.Name == "" || w.RegionID <= 0 {
		return apperr.Invalid("invalid_warehouse", "code, name and region_id are required")
	}
	return nil
}

type Stock struct {
	WarehouseID int64 `json:"warehouse_id"`
	ProductID   int64 `json:"product_id"`
	OnHand      int64 `json:"on_hand"`
	Reserved    int64 `json:"reserved"`
}

func (s Stock) Available() int64 { return s.OnHand - s.Reserved }

// Need is the total base quantity an order requires of one product.
type Need struct {
	ProductID int64
	SKU       string
	BaseQty   int64
}

// AggregateNeeds sums base quantities per product (an order may sell the same
// product by the carton and by the piece) and sorts by product ID. Locking
// stock rows in that fixed order is what prevents deadlocks between
// concurrent reservations.
func AggregateNeeds(lines []events.OrderLine) []Need {
	byProduct := map[int64]*Need{}
	for _, l := range lines {
		n, ok := byProduct[l.ProductID]
		if !ok {
			n = &Need{ProductID: l.ProductID, SKU: l.SKU}
			byProduct[l.ProductID] = n
		}
		n.BaseQty += l.BaseQty
	}
	needs := make([]Need, 0, len(byProduct))
	for _, n := range byProduct {
		needs = append(needs, *n)
	}
	sort.Slice(needs, func(i, j int) bool { return needs[i].ProductID < needs[j].ProductID })
	return needs
}

// Shortages lists the needs the given stock cannot cover. Products with no
// stock row count as zero available. An empty result means the order fits.
func Shortages(needs []Need, stock map[int64]Stock) []events.Shortage {
	var out []events.Shortage
	for _, n := range needs {
		avail := stock[n.ProductID].Available()
		if avail < n.BaseQty {
			out = append(out, events.Shortage{ProductID: n.ProductID, SKU: n.SKU, Requested: n.BaseQty, Available: avail})
		}
	}
	return out
}

// DeliveryStatus is the lifecycle of a delivery order: PENDING → SHIPPED → DELIVERED.
type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "PENDING"
	DeliveryShipped   DeliveryStatus = "SHIPPED"
	DeliveryDelivered DeliveryStatus = "DELIVERED"
)

type DeliveryLine struct {
	LineNo           int             `json:"line_no"`
	ProductID        int64           `json:"product_id"`
	SKU              string          `json:"sku"`
	ProductName      string          `json:"product_name"`
	UoM              string          `json:"uom"`
	ConversionFactor int64           `json:"conversion_factor"`
	OrderedQty       int64           `json:"ordered_qty"`
	DeliveredQty     *int64          `json:"delivered_qty"`
	UnitPrice        decimal.Decimal `json:"unit_price"`
}

type Delivery struct {
	ID              int64          `json:"id"`
	Number          string         `json:"number"`
	OrderID         int64          `json:"order_id"`
	OrderNumber     string         `json:"order_number"`
	WarehouseID     int64          `json:"warehouse_id"`
	CustomerID      int64          `json:"customer_id"`
	CustomerName    string         `json:"customer_name"`
	PaymentTermDays int            `json:"payment_term_days"`
	Status          DeliveryStatus `json:"status"`
	Lines           []DeliveryLine `json:"lines"`
	CreatedAt       time.Time      `json:"created_at"`
	ShippedAt       *time.Time     `json:"shipped_at,omitempty"`
	DeliveredAt     *time.Time     `json:"delivered_at,omitempty"`
}

// NewDelivery creates a PENDING delivery order for a confirmed order.
func NewDelivery(warehouseID int64, o events.OrderConfirmedPayload) Delivery {
	d := Delivery{
		OrderID:         o.OrderID,
		OrderNumber:     o.OrderNumber,
		WarehouseID:     warehouseID,
		CustomerID:      o.CustomerID,
		CustomerName:    o.CustomerName,
		PaymentTermDays: o.PaymentTermDays,
		Status:          DeliveryPending,
	}
	for _, l := range o.Lines {
		d.Lines = append(d.Lines, DeliveryLine{
			LineNo:           l.LineNo,
			ProductID:        l.ProductID,
			SKU:              l.SKU,
			ProductName:      l.ProductName,
			UoM:              l.UoM,
			ConversionFactor: l.ConversionFactor,
			OrderedQty:       l.Qty,
			UnitPrice:        l.UnitPrice,
		})
	}
	return d
}

// Ship marks the goods as having left the warehouse.
func (d *Delivery) Ship(now time.Time) error {
	if d.Status != DeliveryPending {
		return apperr.Conflict("invalid_transition", "delivery %s is %s; only PENDING deliveries can ship", d.Number, d.Status)
	}
	d.Status, d.ShippedAt = DeliveryShipped, &now
	return nil
}

// DeliveredQty overrides how much of one line actually reached the customer.
type DeliveredQty struct {
	LineNo int   `json:"line_no"`
	Qty    int64 `json:"delivered_qty"`
}

// Movement is the stock change delivering one product causes, in base units.
type Movement struct {
	ProductID int64
	Consume   int64 // leaves on_hand: what was handed over
	Release   int64 // leaves reserved: the whole reservation, delivered or not
}

// Deliver records proof of delivery. Lines not listed in overrides are
// delivered in full; a short delivery (damaged or refused goods) lists the
// lower quantity. The reservation is released in full either way, so
// undelivered units return to available stock. It returns the stock
// movements to apply, one per product, in product-ID order.
func (d *Delivery) Deliver(now time.Time, overrides []DeliveredQty) ([]Movement, error) {
	if d.Status != DeliveryShipped {
		return nil, apperr.Conflict("invalid_transition", "delivery %s is %s; only SHIPPED deliveries can be delivered", d.Number, d.Status)
	}

	byLine := map[int]int{}
	for i, l := range d.Lines {
		byLine[l.LineNo] = i
	}
	delivered := make([]int64, len(d.Lines))
	for i, l := range d.Lines {
		delivered[i] = l.OrderedQty
	}
	for _, o := range overrides {
		i, ok := byLine[o.LineNo]
		if !ok {
			return nil, apperr.Invalid("unknown_line", "delivery %s has no line %d", d.Number, o.LineNo)
		}
		if o.Qty < 0 || o.Qty > d.Lines[i].OrderedQty {
			return nil, apperr.Invalid("invalid_quantity", "line %d: delivered_qty must be between 0 and %d", o.LineNo, d.Lines[i].OrderedQty)
		}
		delivered[i] = o.Qty
	}

	anyDelivered := false
	moves := map[int64]*Movement{}
	for i, l := range d.Lines {
		anyDelivered = anyDelivered || delivered[i] > 0
		m, ok := moves[l.ProductID]
		if !ok {
			m = &Movement{ProductID: l.ProductID}
			moves[l.ProductID] = m
		}
		m.Consume += delivered[i] * l.ConversionFactor
		m.Release += l.OrderedQty * l.ConversionFactor
	}
	if !anyDelivered {
		return nil, apperr.Invalid("nothing_delivered", "at least one line must have a delivered quantity; a fully refused delivery is a return, which is not supported yet")
	}

	// Validation passed: only now mutate the delivery.
	for i := range d.Lines {
		q := delivered[i]
		d.Lines[i].DeliveredQty = &q
	}
	d.Status, d.DeliveredAt = DeliveryDelivered, &now
	out := make([]Movement, 0, len(moves))
	for _, m := range moves {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProductID < out[j].ProductID })
	return out, nil
}

// CompletedPayload builds the delivery.completed event.
func (d Delivery) CompletedPayload() events.DeliveryCompletedPayload {
	p := events.DeliveryCompletedPayload{
		DeliveryID:      d.ID,
		DeliveryNumber:  d.Number,
		OrderID:         d.OrderID,
		OrderNumber:     d.OrderNumber,
		CustomerID:      d.CustomerID,
		CustomerName:    d.CustomerName,
		PaymentTermDays: d.PaymentTermDays,
	}
	if d.DeliveredAt != nil {
		p.DeliveredAt = *d.DeliveredAt
	}
	for _, l := range d.Lines {
		var q int64
		if l.DeliveredQty != nil {
			q = *l.DeliveredQty
		}
		p.Lines = append(p.Lines, events.DeliveredLine{
			LineNo:       l.LineNo,
			ProductID:    l.ProductID,
			SKU:          l.SKU,
			ProductName:  l.ProductName,
			UoM:          l.UoM,
			OrderedQty:   l.OrderedQty,
			DeliveredQty: q,
			UnitPrice:    l.UnitPrice,
		})
	}
	return p
}
