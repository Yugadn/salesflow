// Package logger builds the structured JSON logger shared by all services.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON slog.Logger tagged with the service name.
// LOG_LEVEL accepts debug, info, warn or error (default info).
func New(service string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service)
}
