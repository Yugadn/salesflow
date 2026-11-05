package auth

import (
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/yugadn/salesflow/pkg/apperr"
)

// Middleware authenticates every request with a bearer token, except health
// checks and /internal/ service-to-service routes, which are only reachable
// inside the network.
//
// The gateway already rejects bad tokens; services check again because the
// gateway is not the only way onto the network, and because authorization
// rules that depend on data ("is this your order?") live here, not there.
func Middleware(v *Verifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if path == "/healthz" || strings.HasPrefix(path, "/internal/") {
				return next(c)
			}
			header := c.Request().Header.Get(echo.HeaderAuthorization)
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				return apperr.Unauthorized("missing_token", "sign in first: send an Authorization: Bearer token")
			}
			p, err := v.Verify(c.Request().Context(), token)
			if err != nil {
				return apperr.Unauthorized("invalid_token", "your session is invalid or has expired; sign in again")
			}
			c.SetRequest(c.Request().WithContext(WithPrincipal(c.Request().Context(), p)))
			return next(c)
		}
	}
}

// RequireRole allows the route only to callers with one of roles.
func RequireRole(roles ...Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, ok := FromContext(c.Request().Context())
			if !ok {
				return apperr.Unauthorized("missing_token", "sign in first")
			}
			if !p.Is(roles...) {
				return apperr.Forbidden("forbidden", "the %s role cannot do this", p.Role)
			}
			return next(c)
		}
	}
}

// Shorthands for the role sets used across services.
var (
	AnyRole        = RequireRole(RoleAdmin, RoleSales, RoleWarehouse)
	AdminOnly      = RequireRole(RoleAdmin)
	AdminOrSales   = RequireRole(RoleAdmin, RoleSales)
	AdminOrStorage = RequireRole(RoleAdmin, RoleWarehouse)
)
