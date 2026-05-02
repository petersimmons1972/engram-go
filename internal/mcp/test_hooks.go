package mcp

import (
	"context"

	"github.com/petersimmons1972/engram/internal/audit"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/weight"
)

// testHooks groups test-only dependency seams that are injected into Config
// during unit/integration tests. In production Config.testHooks is always nil.
type testHooks struct {
	// auditDB replaces PgPool for audit handlers so tests avoid a real pgxpool.
	auditDB audit.AuditQuerier
	// weightTuner replaces the real TunerWorker in handleMemoryWeightHistory.
	weightTuner *weight.TunerWorker
	// embedProbe replaces embed.NewLiteLLMClient in handleMemoryMigrateEmbedder.
	embedProbe func(ctx context.Context, baseURL, model string) (embed.Client, error)
	// onPostMigrate replaces the weight_config reset block after MigrateEmbedder.
	onPostMigrate func(ctx context.Context, project string)
	// migrateFunc replaces h.Engine.MigrateEmbedder in handleMemoryMigrateEmbedder.
	migrateFunc func(ctx context.Context, model string) (map[string]any, error)
}
