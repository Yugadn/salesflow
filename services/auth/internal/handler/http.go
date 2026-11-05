// Package handler exposes sign-in and user management over HTTP.
package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/services/auth/internal/domain"
	"github.com/yugadn/salesflow/services/auth/internal/usecase"
)

type Handler struct {
	svc      *usecase.Service
	verifier *auth.Verifier
}

func New(svc *usecase.Service, verifier *auth.Verifier) *Handler {
	return &Handler{svc: svc, verifier: verifier}
}

func (h *Handler) Register(e *echo.Echo) {
	// Public: signing in and the public keys verifiers need.
	e.POST("/login", h.login)
	e.GET("/.well-known/jwks.json", h.jwks)

	authed := auth.Middleware(h.verifier)
	e.GET("/me", h.me, authed)
	e.GET("/users", h.listUsers, authed, auth.AdminOnly)
	e.POST("/users", h.createUser, authed, auth.AdminOnly)
	e.PUT("/users/:id", h.updateUser, authed, auth.AdminOnly)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(c echo.Context) error {
	var in loginRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	s, err := h.svc.Login(c.Request().Context(), in.Email, in.Password)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, s)
}

// jwks is served bare (not wrapped in {"data": ...}): verifiers, including
// the KrakenD gateway, expect the standard JWKS document.
func (h *Handler) jwks(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "public, max-age=300")
	return c.JSON(http.StatusOK, h.svc.JWKS())
}

func (h *Handler) me(c echo.Context) error {
	p, _ := auth.FromContext(c.Request().Context())
	u, err := h.svc.Me(c.Request().Context(), p)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, u)
}

func (h *Handler) listUsers(c echo.Context) error {
	out, err := h.svc.ListUsers(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) createUser(c echo.Context) error {
	var in domain.UserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.svc.CreateUser(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, u)
}

func (h *Handler) updateUser(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in domain.UserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	caller, _ := auth.FromContext(c.Request().Context())
	u, err := h.svc.UpdateUser(c.Request().Context(), caller, id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, u)
}
