package config_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/config"
)

// ─── DefaultHost / Host() ──────────────────────────────────────────────────

func TestDefaultHost_Value(t *testing.T) {
	if config.DefaultHost != "127.0.0.1" {
		t.Fatalf("DefaultHost changed: want 127.0.0.1, got %q", config.DefaultHost)
	}
}

func TestHost_Unset(t *testing.T) {
	t.Setenv(config.EnvKeyHost, "")
	if got := config.Host(); got != config.DefaultHost {
		t.Fatalf("unset: want %q, got %q", config.DefaultHost, got)
	}
}

func TestHost_Set(t *testing.T) {
	t.Setenv(config.EnvKeyHost, "0.0.0.0")
	if got := config.Host(); got != "0.0.0.0" {
		t.Fatalf("set: want 0.0.0.0, got %q", got)
	}
}

// ─── DefaultPort / Port() ─────────────────────────────────────────────────

func TestDefaultPort_Value(t *testing.T) {
	if config.DefaultPort != 8788 {
		t.Fatalf("DefaultPort changed: want 8788, got %d", config.DefaultPort)
	}
}

func TestPort_Unset(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "")
	n, err := config.Port()
	if err != nil {
		t.Fatalf("unset: unexpected error: %v", err)
	}
	if n != config.DefaultPort {
		t.Fatalf("unset: want %d, got %d", config.DefaultPort, n)
	}
}

func TestPort_Valid(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "9999")
	n, err := config.Port()
	if err != nil {
		t.Fatalf("valid: unexpected error: %v", err)
	}
	if n != 9999 {
		t.Fatalf("valid: want 9999, got %d", n)
	}
}

func TestPort_Malformed(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "not-a-number")
	_, err := config.Port()
	if err == nil {
		t.Fatal("malformed: expected error, got nil")
	}
}

func TestPort_Zero(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "0")
	_, err := config.Port()
	if err == nil {
		t.Fatal("zero: expected error (non-positive), got nil")
	}
}

func TestPort_Negative(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "-1")
	_, err := config.Port()
	if err == nil {
		t.Fatal("negative: expected error, got nil")
	}
}

// ─── PortOrDefault() ──────────────────────────────────────────────────────

func TestPortOrDefault_Unset(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "")
	if got := config.PortOrDefault(); got != config.DefaultPort {
		t.Fatalf("unset: want %d, got %d", config.DefaultPort, got)
	}
}

func TestPortOrDefault_Valid(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "1234")
	if got := config.PortOrDefault(); got != 1234 {
		t.Fatalf("valid: want 1234, got %d", got)
	}
}

func TestPortOrDefault_Malformed(t *testing.T) {
	t.Setenv(config.EnvKeyPort, "bad")
	if got := config.PortOrDefault(); got != config.DefaultPort {
		t.Fatalf("malformed: want default %d, got %d", config.DefaultPort, got)
	}
}
