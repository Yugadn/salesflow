// Package handler adapts the warehouse usecases to HTTP and RabbitMQ.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
	"github.com/yugadn/salesflow/services/warehouse/internal/usecase"
)

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(e *echo.Echo) {
	e.POST("/warehouses", h.createWarehouse)
	e.GET("/warehouses", h.listWarehouses)
	e.PUT("/warehouses/:id", h.updateWarehouse)
	e.POST("/warehouses/:id/receipts", h.receive)
	e.GET("/warehouses/:id/stocks", h.listStocks)

	e.GET("/deliveries", h.listDeliveries)
	e.GET("/deliveries/:id", h.getDelivery)
	e.POST("/deliveries/:id/ship", h.ship)
	e.POST("/deliveries/:id/deliver", h.deliver)
}

func (h *Handler) createWarehouse(c echo.Context) error {
	var in domain.Warehouse
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateWarehouse(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listWarehouses(c echo.Context) error {
	out, err := h.svc.ListWarehouses(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) updateWarehouse(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in domain.Warehouse
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.UpdateWarehouse(c.Request().Context(), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

type receiptRequest struct {
	ProductID int64 `json:"product_id"`
	Qty       int64 `json:"qty"`
}

func (h *Handler) receive(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in receiptRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.ReceiveStock(c.Request().Context(), id, in.ProductID, in.Qty)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) listStocks(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.ListStocks(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) listDeliveries(c echo.Context) error {
	f := domain.DeliveryFilter{Status: domain.DeliveryStatus(strings.ToUpper(c.QueryParam("status")))}
	f.Limit, _ = strconv.Atoi(c.QueryParam("limit"))
	f.OrderID, _ = strconv.ParseInt(c.QueryParam("order_id"), 10, 64)
	out, err := h.svc.ListDeliveries(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) getDelivery(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.GetDelivery(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) ship(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.Ship(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

type deliverRequest struct {
	// Optional. Lines left out are delivered in full.
	Lines []domain.DeliveredQty `json:"lines"`
}

func (h *Handler) deliver(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in deliverRequest
	if c.Request().ContentLength != 0 {
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
	}
	out, err := h.svc.Deliver(c.Request().Context(), id, in.Lines)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

// Events adapts HandleEvent to the consumer; application errors are permanent.
func Events(svc *usecase.Service) rabbit.Handler {
	return func(ctx context.Context, env events.Envelope) error {
		err := svc.HandleEvent(ctx, env)
		if _, ok := apperr.As(err); ok {
			return rabbit.Permanent(err)
		}
		return err
	}
}
