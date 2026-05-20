// Package config is the single source of truth for ENGRAM_* environment
// variable names and their default values. All defaults live here — not in
// docker-compose files, not in k8s ConfigMaps, and not scattered across
// cmd/ helpers.
//
// Deployment files (docker-compose, k8s) may OVERRIDE values by setting the
// corresponding environment variable; they must not carry defaults that differ
// from the ones defined here.
//
// Usage:
//
//	port := config.DefaultPort  // canonical integer default
//	host := config.EnvHost()    // reads ENGRAM_HOST, falls back to DefaultHost
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ─── Canonical env-var names ───────────────────────────────────────────────

const (
	EnvKeyHost = "ENGRAM_HOST"
	EnvKeyPort = "ENGRAM_PORT"
)

// ─── Canonical default values ──────────────────────────────────────────────
//
// These are the ONLY authoritative defaults. If docker-compose or a k8s
// ConfigMap carries a value that differs from these, that deployment file is
// wrong and should be corrected.

const (
	// DefaultHost is the bind address used when ENGRAM_HOST is unset.
	//
	// Inside a Docker container, ENGRAM_HOST should be overridden to "0.0.0.0"
	// so docker-proxy can forward traffic from the host loopback to the
	// container's eth0 interface. The host port-mapping (127.0.0.1:8788:8788)
	// already restricts external access — the container-side bind does not
	// increase the security surface. See #666, #728.
	//
	// On a bare-metal or VM install, the loopback default is intentional.
	DefaultHost = "127.0.0.1"

	// DefaultPort is the MCP SSE port used when ENGRAM_PORT is unset.
	DefaultPort = 8788
)

// ─── Typed accessors ───────────────────────────────────────────────────────

// Host reads ENGRAM_HOST, returning DefaultHost when the variable is unset or
// empty.
func Host() string {
	if v := os.Getenv(EnvKeyHost); v != "" {
		return v
	}
	return DefaultHost
}

// Port reads ENGRAM_PORT as a base-10 integer, returning DefaultPort when the
// variable is unset or empty. Returns an error when the value is set but
// cannot be parsed as a positive integer.
func Port() (int, error) {
	v := os.Getenv(EnvKeyPort)
	if v == "" {
		return DefaultPort, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("ENGRAM_PORT %q: must be a positive integer", v)
	}
	return n, nil
}

// PortOrDefault returns the port from ENGRAM_PORT, falling back to DefaultPort
// on any error (including unparseable values). Callers that need strict
// validation should use Port() instead.
func PortOrDefault() int {
	n, err := Port()
	if err != nil {
		return DefaultPort
	}
	return n
}
