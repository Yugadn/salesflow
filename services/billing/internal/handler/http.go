// Package handler adapts the billing usecases to HTTP and RabbitMQ.
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/services/billing/internal/domain"
	"github.com/yugadn/salesflow/services/billing/internal/usecase"
)

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(e *echo.Echo) {
	e.GET("/invoices", h.list, auth.AdminOnly)
	e.GET("/invoices/:id", h.get, auth.AdminOnly)
}

func (h *Handler) list(c echo.Context) error {
	var f domain.InvoiceFilter
	f.CustomerID, _ = strconv.ParseInt(c.QueryParam("customer_id"), 10, 64)
	f.OrderID, _ = strconv.ParseInt(c.QueryParam("order_id"), 10, 64)
	f.Limit, _ = strconv.Atoi(c.QueryParam("limit"))
	out, err := h.svc.List(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) get(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), id)
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
