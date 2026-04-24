// Package testutil provides shared helpers for integration tests that require
// a live PostgreSQL instance.
package testutil

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// DSN returns the TEST_DATABASE_URL environment variable, skipping t if unset.
func DSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// UniqueProject returns a collision-free project name for each test run.
func UniqueProject(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}
