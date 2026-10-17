// Package handler exposes master data over HTTP.
package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/services/master/internal/domain"
	"github.com/yugadn/salesflow/services/master/internal/usecase"
)

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Register mounts all routes on e.
func (h *Handler) Register(e *echo.Echo) {
	e.POST("/regions", h.createRegion)
	e.GET("/regions", h.listRegions)

	e.POST("/customers", h.createCustomer)
	e.GET("/customers", h.listCustomers)
	e.GET("/customers/:id", h.getCustomer)

	e.POST("/salespersons", h.createSalesperson)
	e.GET("/salespersons", h.listSalespersons)

	e.POST("/uoms", h.createUoM)
	e.GET("/uoms", h.listUoMs)

	e.POST("/products", h.createProduct)
	e.GET("/products", h.listProducts)
	e.GET("/products/:id", h.getProduct)
	e.PUT("/products/:id/uoms", h.setConversion)
	e.PUT("/products/:id/prices", h.setPrice)
	e.GET("/products/:id/prices", h.listPrices)

	// Service-to-service endpoint used by order-service.
	e.POST("/internal/quote", h.quote)
}

func (h *Handler) createRegion(c echo.Context) error {
	var in domain.Region
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateRegion(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listRegions(c echo.Context) error {
	out, err := h.svc.ListRegions(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) createCustomer(c echo.Context) error {
	var in domain.Customer
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateCustomer(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listCustomers(c echo.Context) error {
	out, err := h.svc.ListCustomers(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) getCustomer(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.GetCustomer(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) createSalesperson(c echo.Context) error {
	var in domain.Salesperson
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateSalesperson(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listSalespersons(c echo.Context) error {
	out, err := h.svc.ListSalespersons(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) createUoM(c echo.Context) error {
	var in domain.UoM
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateUoM(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listUoMs(c echo.Context) error {
	out, err := h.svc.ListUoMs(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) createProduct(c echo.Context) error {
	var in domain.Product
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateProduct(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusCreated, out)
}

func (h *Handler) listProducts(c echo.Context) error {
	out, err := h.svc.ListProducts(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) getProduct(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.GetProduct(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) setConversion(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in domain.ProductUoM
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SetConversion(c.Request().Context(), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) setPrice(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in domain.Price
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	in.ProductID = id
	out, err := h.svc.SetPrice(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) listPrices(c echo.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.ListPrices(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}

func (h *Handler) quote(c echo.Context) error {
	var in domain.QuoteRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Quote(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(c, http.StatusOK, out)
}
