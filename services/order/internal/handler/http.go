// Package handler adapts the order usecases to HTTP and RabbitMQ.
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
	"github.com/yugadn/salesflow/services/order/internal/domain"
	"github.com/yugadn/salesflow/services/order/internal/usecase"
)

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(e *echo.Echo) {
	e.POST("/orders", h.create)
	e.GET("/orders", h.list)
	e.GET("/orders/:id", h.get)
	e.POST("/orders/:id/confirm", h.confirm)
	e.POST("/orders/:id/cancel", h.cancel)
}

func (h *Handler) create(c echo.Context) error {
	var in usecase.CreateInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.svc.Create(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, o)
}

func (h *Handler) list(c echo.Context) error {
	status := domain.Status(strings.ToUpper(c.QueryParam("status")))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	out, err := h.svc.List(c.Request().Context(), status, limit)
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
	o, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, o)
}

func (h *Handler) confirm(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	o, err := h.svc.Confirm(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, o)
}

func (h *Handler) cancel(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	o, err := h.svc.Cancel(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, o)
}

// Events adapts HandleEvent to the consumer. Application errors cannot be
// fixed by retrying, so they are dead-lettered straight away.
func Events(svc *usecase.Service) rabbit.Handler {
	return func(ctx context.Context, env events.Envelope) error {
		err := svc.HandleEvent(ctx, env)
		if _, ok := apperr.As(err); ok {
			return rabbit.Permanent(err)
		}
		return err
	}
}
