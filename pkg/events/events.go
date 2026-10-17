// Package events is the contract between services: the exchange, routing
// keys, the envelope every message travels in, and each payload's shape.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Exchange is the topic exchange all domain events are published to.
const Exchange = "salesflow.events"

// Routing keys. Each event is named after the fact that happened, and is
// published only by the service that owns that fact.
const (
	OrderConfirmed         = "order.confirmed"          // order-service
	DeliveryCreated        = "delivery.created"         // warehouse-service: stock reserved
	StockReservationFailed = "stock.reservation_failed" // warehouse-service
	DeliveryCompleted      = "delivery.completed"       // warehouse-service
	InvoiceCreated         = "invoice.created"          // billing-service
)

// Envelope wraps every payload. ID is the idempotency key consumers dedupe on.
type Envelope struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

// New builds an envelope with a fresh ID around payload.
func New(source, eventType string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	return Envelope{
		ID:         uuid.NewString(),
		Type:       eventType,
		Source:     source,
		OccurredAt: time.Now().UTC(),
		Payload:    raw,
	}, nil
}

// Decode unmarshals the payload into v.
func (e Envelope) Decode(v any) error {
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return fmt.Errorf("decode %s payload: %w", e.Type, err)
	}
	return nil
}

// OrderLine is an order line snapshot. Prices are fixed at order time so
// downstream services never re-read master data.
type OrderLine struct {
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

// OrderConfirmedPayload is published when a sales order is confirmed.
type OrderConfirmedPayload struct {
	OrderID         int64           `json:"order_id"`
	OrderNumber     string          `json:"order_number"`
	CustomerID      int64           `json:"customer_id"`
	CustomerName    string          `json:"customer_name"`
	RegionID        int64           `json:"region_id"`
	PaymentTermDays int             `json:"payment_term_days"`
	Total           decimal.Decimal `json:"total"`
	Lines           []OrderLine     `json:"lines"`
}

// DeliveryCreatedPayload is published once stock is reserved for an order.
type DeliveryCreatedPayload struct {
	DeliveryID     int64  `json:"delivery_id"`
	DeliveryNumber string `json:"delivery_number"`
	OrderID        int64  `json:"order_id"`
	WarehouseID    int64  `json:"warehouse_id"`
}

// Shortage describes one product that could not be reserved, in base units.
type Shortage struct {
	ProductID int64  `json:"product_id"`
	SKU       string `json:"sku"`
	Requested int64  `json:"requested"`
	Available int64  `json:"available"`
}

// StockReservationFailedPayload is published when an order cannot be fulfilled.
type StockReservationFailedPayload struct {
	OrderID   int64      `json:"order_id"`
	Reason    string     `json:"reason"`
	Shortages []Shortage `json:"shortages,omitempty"`
}

// DeliveredLine reports what actually reached the customer for one order line.
type DeliveredLine struct {
	LineNo       int             `json:"line_no"`
	ProductID    int64           `json:"product_id"`
	SKU          string          `json:"sku"`
	ProductName  string          `json:"product_name"`
	UoM          string          `json:"uom"`
	OrderedQty   int64           `json:"ordered_qty"`
	DeliveredQty int64           `json:"delivered_qty"`
	UnitPrice    decimal.Decimal `json:"unit_price"`
}

// DeliveryCompletedPayload is published when goods are delivered (proof of delivery).
type DeliveryCompletedPayload struct {
	DeliveryID      int64           `json:"delivery_id"`
	DeliveryNumber  string          `json:"delivery_number"`
	OrderID         int64           `json:"order_id"`
	OrderNumber     string          `json:"order_number"`
	CustomerID      int64           `json:"customer_id"`
	CustomerName    string          `json:"customer_name"`
	PaymentTermDays int             `json:"payment_term_days"`
	DeliveredAt     time.Time       `json:"delivered_at"`
	Lines           []DeliveredLine `json:"lines"`
}

// InvoiceCreatedPayload is published when billing issues an invoice.
type InvoiceCreatedPayload struct {
	InvoiceID     int64           `json:"invoice_id"`
	InvoiceNumber string          `json:"invoice_number"`
	OrderID       int64           `json:"order_id"`
	DeliveryID    int64           `json:"delivery_id"`
	Total         decimal.Decimal `json:"total"`
	DueDate       string          `json:"due_date"` // YYYY-MM-DD
}
