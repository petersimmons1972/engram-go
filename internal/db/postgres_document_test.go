package db

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests exercise the raw document storage layer. They require a live
// PostgreSQL reachable via DATABASE_URL; skip politely when unset so the
// unit-test build stays green on machines without a DB.

func newTestBackend(t *testing.T) *PostgresBackend {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	b, err := NewPostgresBackend(ctx, "a4-test", dsn)
	require.NoError(t, err)
	t.Cleanup(b.Close)
	return b
}

func TestStoreDocument_Roundtrip(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	body := "hello tier-2 world\n" // arbitrary content
	id, err := b.StoreDocument(ctx, "a4-test", body)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := b.GetDocument(ctx, id)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestStoreDocument_EmptyContentRejected(t *testing.T) {
	b := newTestBackend(t)
	_, err := b.StoreDocument(context.Background(), "a4-test", "")
	require.Error(t, err)
}

func TestGetDocument_Missing(t *testing.T) {
	b := newTestBackend(t)
	got, err := b.GetDocument(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	require.Equal(t, "", got)
}
