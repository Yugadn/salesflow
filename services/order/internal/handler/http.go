// Package handler adapts the order usecases to HTTP and RabbitMQ.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
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
	read, sell := auth.AnyRole, auth.AdminOrSales

	e.POST("/orders", h.create, sell)
	e.GET("/orders", h.list, read)
	e.GET("/orders/:id", h.get, read)
	e.POST("/orders/:id/confirm", h.confirm, sell)
	e.POST("/orders/:id/cancel", h.cancel, sell)
}

func (h *Handler) create(c echo.Context) error {
	var in usecase.CreateInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, replayed, err := h.svc.CreateIdempotent(c.Request().Context(), principal(c), c.Request().Header.Get("Idempotency-Key"), in)
	if err != nil {
		return err
	}
	if replayed {
		// Same key, same body: hand back the order the first request created.
		c.Response().Header().Set("Idempotent-Replayed", "true")
		return httpx.JSON(c, http.StatusOK, o)
	}
	return httpx.JSON(c, http.StatusCreated, o)
}

func (h *Handler) list(c echo.Context) error {
	f := usecase.ListFilter{Status: domain.Status(strings.ToUpper(c.QueryParam("status")))}
	f.Limit, _ = strconv.Atoi(c.QueryParam("limit"))
	out, err := h.svc.List(c.Request().Context(), principal(c), f)
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
	o, err := h.svc.Get(c.Request().Context(), principal(c), id)
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
	o, err := h.svc.Confirm(c.Request().Context(), principal(c), id)
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
	o, err := h.svc.Cancel(c.Request().Context(), principal(c), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, o)
}

// principal is the caller the auth middleware attached to the request.
func principal(c echo.Context) auth.Principal {
	p, _ := auth.FromContext(c.Request().Context())
	return p
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
