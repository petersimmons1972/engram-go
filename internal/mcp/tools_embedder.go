package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/netutil"
)

func handleMemoryMigrateEmbedder(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, newModel := requireString(args, "new_model")
	if errResult != nil {
		return errResult, nil
	}

	// Resolve the Ollama URL for this operation: caller may supply ollama_url to
	// override the server default; if present, apply the same SSRF guard used at
	// startup (#291). Only literal private IPs are blocked — hostnames are allowed
	// because they resolve to container IPs by design and are not attacker-controlled.
	ollamaURL := cfg.LiteLLMURL
	if raw := getString(args, "ollama_url", ""); raw != "" {
		parsed, parseErr := url.ParseRequestURI(raw)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("invalid ollama_url %q: must be an http:// or https:// URL", raw)
		}
		if host := parsed.Hostname(); net.ParseIP(host) != nil && netutil.IsPrivateIP(host) {
			return nil, fmt.Errorf("invalid ollama_url: IP %q is in a private/reserved range (SSRF protection)", host)
		}
		ollamaURL = raw
	}

	// Dimension pre-flight (#251): compare stored dims against the new model's output.
	// Avoids nulling all embeddings only to discover a dimension mismatch at INSERT.
	if storedDimsStr, ok, metaErr := h.Engine.Backend().GetMeta(ctx, project, "embedder_dimensions"); metaErr == nil && ok && storedDimsStr != "" {
		var probeFunc func(ctx context.Context, baseURL, model string) (embed.Client, error)
		if cfg.testHooks != nil {
			probeFunc = cfg.testHooks.embedProbe
		}
		if probeFunc == nil {
			targetDims := cfg.EmbedDimensions
			probeFunc = func(ctx context.Context, baseURL, model string) (embed.Client, error) {
				return embed.NewLiteLLMClient(ctx, baseURL, model, "", targetDims)
			}
		}
		probeClient, probeErr := probeFunc(ctx, ollamaURL, newModel)
		if probeErr != nil {
			return nil, fmt.Errorf("cannot verify new embedder model dimensions: %w", probeErr)
		}
		newDims := probeClient.Dimensions()
		var storedDims int
		if _, scanErr := fmt.Sscanf(storedDimsStr, "%d", &storedDims); scanErr == nil && storedDims > 0 {
			if newDims != storedDims {
				return nil, fmt.Errorf(
					"dimension mismatch: current model stores %d-dim vectors, new model %q produces %d-dim vectors — pgvector column must be rebuilt first",
					storedDims, newModel, newDims,
				)
			}
		}
	}

	var result map[string]any
	if cfg.testHooks != nil && cfg.testHooks.migrateFunc != nil {
		result, err = cfg.testHooks.migrateFunc(ctx, newModel)
	} else {
		result, err = h.Engine.MigrateEmbedder(ctx, newModel)
	}
	if err != nil {
		return nil, err
	}

	// Reset weight_config to defaults for this project: learned weights are
	// no longer valid after the embedding model changes (#Phase4).
	// Best-effort — a failure here does not roll back the migration.
	// A history row is inserted before deletion so the reset is auditable.
	if cfg.testHooks != nil && cfg.testHooks.onPostMigrate != nil {
		cfg.testHooks.onPostMigrate(ctx, project)
	} else if cfg.PgPool != nil {
		histID := uuid.New().String()
		if _, histErr := cfg.PgPool.Exec(ctx,
			`INSERT INTO weight_history (id, project, applied_at, weight_vector, weight_bm25, weight_recency, weight_precision, notes, trigger_data)
			 VALUES ($1, $2, NOW(), 0.45, 0.30, 0.10, 0.15, 'reset on embedder migration', '{"reason":"embedder_migration"}'::jsonb)`,
			histID, project,
		); histErr != nil {
			slog.Warn("memory_migrate_embedder: weight_history insert failed",
				"project", project, "err", histErr)
		}
		if _, delErr := cfg.PgPool.Exec(ctx,
			`DELETE FROM weight_config WHERE project = $1`, project); delErr != nil {
			slog.Warn("memory_migrate_embedder: weight_config reset failed",
				"project", project, "err", delErr)
		} else {
			slog.Info("weight_config reset after embedder migration", "project", project)
		}
	}

	return toolResult(result)
}

// handleMemoryExportAll exports all memories to markdown files in output_path.

func handleMemoryModels(ctx context.Context, _ *EnginePool, _ mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	installed, err := fetchLiteLLMModels(ctx, cfg.LiteLLMURL)
	if err != nil {
		// Non-fatal: return registry with installed=false for all entries.
		installed = map[string]bool{}
	}

	type modelEntry struct {
		Name        string `json:"name"`
		Dimensions  int    `json:"dimensions"`
		SizeMB      int    `json:"size_mb"`
		Description string `json:"description"`
		Recommended bool   `json:"recommended"`
		Installed   bool   `json:"installed"`
	}

	suggested := make([]modelEntry, 0, len(embed.SuggestedModels))
	for _, s := range embed.SuggestedModels {
		suggested = append(suggested, modelEntry{
			Name:        s.Name,
			Dimensions:  s.Dimensions,
			SizeMB:      s.SizeMB,
			Description: s.Description,
			Recommended: s.Recommended,
			Installed:   installed[s.Name] || installed[s.Name+":latest"],
		})
	}

	installedList := make([]string, 0, len(installed))
	for name := range installed {
		installedList = append(installedList, name)
	}
	sort.Strings(installedList)

	return toolResult(map[string]any{
		"current":   cfg.EmbedModel,
		"installed": installedList,
		"suggested": suggested,
	})
}

// fetchLiteLLMModels calls GET /v1/models and returns a set of model IDs.
func fetchLiteLLMModels(ctx context.Context, baseURL string) (map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(result.Data))
	for _, m := range result.Data {
		names[m.ID] = true
	}
	return names, nil
}

// cosineSim32 computes cosine similarity between two float32 vectors.
// Returns 0.0 if either vector is zero-magnitude or lengths differ.
func cosineSim32(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// evalProbeSentences is the fixed probe set used by memory_embedding_eval.
var evalProbeSentences = []string{
	"deploy kubernetes cluster",
	"rollback failed deployment",
	"database migration failed",
	"postgres connection refused",
	"memory recall returned empty",
	"the quick brown fox jumps",
	"unrelated topic about cooking",
}

// handleMemoryEmbeddingEval compares two Ollama embedding models by embedding
// evalProbeSentences with each model and reporting mean pairwise cosine
// similarity. Lower mean similarity = better semantic separation.
// model_b defaults to the recommended model in embed.SuggestedModels.
func handleMemoryEmbeddingEval(ctx context.Context, _ *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	defaultModelA := cfg.EmbedModel
	if defaultModelA == "" {
		defaultModelA = "nomic-embed-text"
	}
	modelA := getString(args, "model_a", defaultModelA)
	modelB := getString(args, "model_b", "")
	if modelB == "" {
		if rec := embed.DefaultRecommendedModel(); rec != nil {
			modelB = rec.Name
		} else {
			modelB = "mxbai-embed-large"
		}
	}
	if modelA == modelB {
		return nil, fmt.Errorf("memory_embedding_eval: model_a and model_b must differ")
	}

	clientA := embed.NewLiteLLMClientNoProbe(cfg.LiteLLMURL, modelA, "", cfg.EmbedDimensions)
	clientB := embed.NewLiteLLMClientNoProbe(cfg.LiteLLMURL, modelB, "", cfg.EmbedDimensions)

	type embedResult struct {
		sentence string
		vec      []float32
	}
	embedAll := func(c embed.Client) ([]embedResult, error) {
		results := make([]embedResult, 0, len(evalProbeSentences))
		for _, s := range evalProbeSentences {
			// 2s deadline — Ollama must never block MCP calls.
			embedCtx, embedCancel := context.WithTimeout(context.Background(), 2*time.Second)
			vec, err := c.Embed(embedCtx, s)
			embedCancel()
			if err != nil {
				return nil, fmt.Errorf("embed %q: %w", s, err)
			}
			results = append(results, embedResult{sentence: s, vec: vec})
		}
		return results, nil
	}

	vecsA, err := embedAll(clientA)
	if err != nil {
		return nil, fmt.Errorf("memory_embedding_eval: model_a embeddings: %w", err)
	}
	vecsB, err := embedAll(clientB)
	if err != nil {
		return nil, fmt.Errorf("memory_embedding_eval: model_b embeddings: %w", err)
	}

	meanSim := func(vecs []embedResult) float64 {
		if len(vecs) < 2 {
			return 0
		}
		var total float64
		count := 0
		for i := 0; i < len(vecs); i++ {
			for j := i + 1; j < len(vecs); j++ {
				total += cosineSim32(vecs[i].vec, vecs[j].vec)
				count++
			}
		}
		return total / float64(count)
	}

	simA := meanSim(vecsA)
	simB := meanSim(vecsB)

	recommendation := modelA
	if simB < simA {
		recommendation = modelB
	}

	return toolResult(map[string]any{
		"model_a": map[string]any{
			"name":                 modelA,
			"dimensions":           clientA.Dimensions(),
			"mean_pairwise_cosine": simA,
		},
		"model_b": map[string]any{
			"name":                 modelB,
			"dimensions":           clientB.Dimensions(),
			"mean_pairwise_cosine": simB,
		},
		"recommendation": recommendation,
		"reason":         "lower mean pairwise similarity indicates better semantic separation",
		"note":           "This comparison uses probe sentences only. Run memory_migrate_embedder to apply the chosen model to stored embeddings.",
		"probe_count":    len(evalProbeSentences),
	})
}
