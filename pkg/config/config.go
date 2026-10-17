// Package config reads service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"time"
)

// String returns the value of key, or def when it is unset or empty.
func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// MustString returns the value of key and fails fast when it is missing,
// so a misconfigured container stops at boot instead of on first request.
func MustString(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %s is not set", key))
	}
	return v
}

// Duration parses key as a Go duration (e.g. "2s"), falling back to def.
func Duration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("config: %s=%q is not a valid duration: %v", key, v, err))
	}
	return d
}
