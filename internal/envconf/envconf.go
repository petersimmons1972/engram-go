// Package envconf provides helpers for reading ENGRAM_* environment variables.
// All functions return the provided default when the variable is unset or empty.
// Parse failures log a slog.Warn and return the default.
package envconf

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Float reads name as a float64. Returns def if unset/empty; warns and returns
// def if the value is malformed.
func Float(name string, def float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		slog.Warn(name+": invalid float, using default", "value", raw, "default", def)
		return def
	}
	return v
}

// FloatBounded is like Float but rejects values outside [lo, hi] with a warning.
func FloatBounded(name string, def, lo, hi float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		slog.Warn(name+": invalid float, using default", "value", raw, "default", def)
		return def
	}
	if v < lo || v > hi {
		slog.Warn(name+": out of range, using default", "value", v, "min", lo, "max", hi, "default", def)
		return def
	}
	return v
}

// Int reads name as a base-10 integer. Returns def if unset/empty; warns and
// returns def if the value is malformed.
func Int(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		slog.Warn(name+": invalid integer, using default", "value", raw, "default", def)
		return def
	}
	return v
}

// String reads name as a string. Returns def if unset/empty.
func String(name string, def string) string {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	return raw
}

// DurationHours reads name as a float64 number of hours and returns a
// time.Duration. Returns def if unset/empty or if the value is malformed or
// non-positive.
func DurationHours(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		slog.Warn(name+": invalid float, using default", "value", raw, "default", def)
		return def
	}
	if v <= 0 {
		slog.Warn(name+": must be positive, using default", "value", v, "default", def)
		return def
	}
	return time.Duration(float64(time.Hour) * v)
}
