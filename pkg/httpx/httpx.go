// Package httpx holds the HTTP conventions shared by all services: server
// setup, the response envelope, and mapping application errors to statuses.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yugadn/salesflow/pkg/apperr"
)

// NewServer returns an Echo instance with recovery, request IDs, structured
// request logging and the shared error handler installed.
func NewServer(log *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler(log)
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		// Container health probes hit /healthz every few seconds; logging them buries real traffic.
		Skipper:      func(c echo.Context) bool { return c.Path() == "/healthz" },
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
			log.Info("http request",
				"method", v.Method, "uri", v.URI, "status", v.Status,
				"latency_ms", v.Latency.Milliseconds(), "request_id", v.RequestID)
			return nil
		},
	}))
	return e
}

// Serve runs e on addr until ctx is cancelled, then shuts down gracefully.
func Serve(ctx context.Context, e *echo.Echo, addr string) error {
	errCh := make(chan error, 1)
	go func() {
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return e.Shutdown(shutdownCtx)
	}
}

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

// RegisterHealth mounts GET /healthz, which reports 503 when the database is
// unreachable so orchestrators stop routing to the instance.
func RegisterHealth(e *echo.Echo, db Pinger) {
	e.GET("/healthz", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
}

// HealthcheckIfRequested handles `<binary> healthcheck`: it probes the local
// /healthz and exits 0 or 1. Distroless images have no curl, so the service
// binary doubles as its own Docker HEALTHCHECK probe.
func HealthcheckIfRequested(addr string) {
	if len(os.Args) < 2 || os.Args[1] != "healthcheck" {
		return
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + "/healthz")
	if err != nil {
		os.Exit(1)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}

// JSON writes {"data": data} with status.
func JSON(c echo.Context, status int, data any) error {
	return c.JSON(status, map[string]any{"data": data})
}

// Bind decodes the request body into v, reporting malformed input as 400.
func Bind(c echo.Context, v any) error {
	if err := (&echo.DefaultBinder{}).BindBody(c, v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "malformed request body")
	}
	return nil
}

// ParamID parses the path parameter name as a positive int64.
func ParamID(c echo.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Invalid("invalid_id", "%s must be a positive integer", name)
	}
	return id, nil
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func errorHandler(log *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status, body := http.StatusInternalServerError, errorBody{Code: "internal", Message: "internal server error"}

		var he *echo.HTTPError
		if ae, ok := apperr.As(err); ok {
			body = errorBody{Code: ae.Code, Message: ae.Message}
			switch ae.Kind {
			case apperr.KindInvalid:
				status = http.StatusUnprocessableEntity
			case apperr.KindNotFound:
				status = http.StatusNotFound
			case apperr.KindConflict:
				status = http.StatusConflict
			}
		} else if errors.As(err, &he) {
			status = he.Code
			body = errorBody{Code: http.StatusText(he.Code), Message: http.StatusText(he.Code)}
			if msg, ok := he.Message.(string); ok {
				body.Message = msg
			}
		}

		if status >= http.StatusInternalServerError {
			log.Error("request failed", "err", err, "uri", c.Request().RequestURI)
		}
		_ = c.JSON(status, map[string]any{"error": body})
	}
}
