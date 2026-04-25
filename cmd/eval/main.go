// engram-eval runs golden-set retrieval evaluation against a live engram-go MCP server.
// Usage: engram-eval -golden golden.json -k 5 -project default
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/petersimmons1972/engram/internal/eval"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)"
var Version = "dev"

type goldenEntry struct {
	Query       string   `json:"query"`
	RelevantIDs []string `json:"relevant_ids"`
}

// recallResponse mirrors the JSON shape returned by memory_recall.
type recallResponse struct {
	Results []struct {
		Memory struct {
			ID string `json:"id"`
		} `json:"memory"`
		Score float64 `json:"score"`
	} `json:"results"`
	Count int `json:"count"`
}

func main() {
	goldenFile := flag.String("golden", "", "Path to golden set JSON file (required)")
	k := flag.Int("k", 5, "k for precision@k and NDCG@k")
	project := flag.String("project", "default", "Engram project name to query")
	outputFile := flag.String("output", "", "Write baseline summary to this file (optional)")
	urlFlag := flag.String("url", "", "Override ENGRAM_URL env var")
	versionFlag := flag.Bool("version", false, "print version and exit")
	outputJSON := flag.Bool("output-json", false, "emit summary as JSON to stdout; send per-query progress to stderr")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("engram-eval %s\n", Version)
		os.Exit(0)
	}

	if *goldenFile == "" {
		fmt.Fprintf(os.Stderr, "error: --golden is required\n")
		os.Exit(2)
	}

	// Resolve server URL and credentials — flags beat env vars.
	serverURL := envOr("ENGRAM_URL", "http://localhost:8788")
	if *urlFlag != "" {
		serverURL = *urlFlag
	}
	apiKey := os.Getenv("ENGRAM_API_KEY")
	provider := envOr("ENGRAM_EMBED_PROVIDER", "ollama")

	// Load golden set.
	data, err := os.ReadFile(*goldenFile)
	if err != nil {
		log.Fatalf("read golden file: %v", err)
	}
	var golden []goldenEntry
	if err := json.Unmarshal(data, &golden); err != nil {
		log.Fatalf("parse golden file: %v", err)
	}
	if len(golden) == 0 {
		fmt.Fprintf(os.Stderr, "error: golden set is empty\n")
		os.Exit(2)
	}

	// Connect to MCP server via SSE.
	sseURL := strings.TrimRight(serverURL, "/") + "/sse"

	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	mcpClient, err := client.NewSSEMCPClient(sseURL, transport.WithHeaders(headers))
	if err != nil {
		log.Fatalf("create SSE client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		log.Fatalf("start SSE client: %v", err)
	}

	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "engram-eval",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		log.Fatalf("initialize MCP session: %v", err)
	}

	// Run evaluation.
	total := len(golden)
	var sumP, sumMRR, sumNDCG float64

	// progressf writes per-query progress lines. When --output-json is set, progress
	// goes to stderr so stdout carries only the machine-readable JSON result.
	progressf := func(format string, args ...any) {
		if *outputJSON {
			fmt.Fprintf(os.Stderr, format, args...)
		} else {
			fmt.Printf(format, args...)
		}
	}

	for i, entry := range golden {
		ids, callErr := recallIDs(ctx, mcpClient, entry.Query, *project, *k)
		if callErr != nil {
			// Log warning and count this query as all-zeros — do not abort.
			log.Printf("WARN [%d/%d] memory_recall failed for %q: %v", i+1, total, entry.Query, callErr)
			progressf("[%d/%d] %q -> P@%d=0.00 MRR=0.00 NDCG@%d=0.00  (recall error)\n",
				i+1, total, entry.Query, *k, *k)
			continue
		}

		relevantSet := make(map[string]bool, len(entry.RelevantIDs))
		for _, id := range entry.RelevantIDs {
			relevantSet[id] = true
		}

		p := eval.PrecisionAtK(ids, relevantSet, *k)
		mrr := eval.MRR(ids, relevantSet)
		ndcg := eval.NDCG(ids, relevantSet, *k)

		sumP += p
		sumMRR += mrr
		sumNDCG += ndcg

		progressf("[%d/%d] %q -> P@%d=%.2f MRR=%.2f NDCG@%d=%.2f\n",
			i+1, total, entry.Query, *k, p, mrr, *k, ndcg)
	}

	avgP := sumP / float64(total)
	avgMRR := sumMRR / float64(total)
	avgNDCG := sumNDCG / float64(total)

	if *outputJSON {
		// Emit machine-readable summary to stdout; human-readable prose omitted.
		type jsonSummary struct {
			AvgPrecisionAtK float64 `json:"avg_precision_at_k"`
			AvgMRR          float64 `json:"avg_mrr"`
			AvgNDCGAtK      float64 `json:"avg_ndcg_at_k"`
			K               int     `json:"k"`
			TotalQueries    int     `json:"total_queries"`
		}
		out, _ := json.Marshal(jsonSummary{
			AvgPrecisionAtK: avgP,
			AvgMRR:          avgMRR,
			AvgNDCGAtK:      avgNDCG,
			K:               *k,
			TotalQueries:    total,
		})
		fmt.Printf("%s\n", out)
		return
	}

	summary := buildSummary(total, *k, avgP, avgMRR, avgNDCG, provider, *outputFile)
	fmt.Println(summary)

	if *outputFile != "" {
		if writeErr := os.WriteFile(*outputFile, []byte(summary), 0o644); writeErr != nil {
			log.Printf("WARN failed to write output file %q: %v", *outputFile, writeErr)
		}
	}
}

// recallIDs calls memory_recall and returns the ordered list of memory IDs.
func recallIDs(ctx context.Context, c *client.Client, query, project string, k int) ([]string, error) {
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_recall",
			Arguments: map[string]any{
				"query":   query,
				"project": project,
				"top_k":   k,
				"detail":  "summary",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CallTool: %w", err)
	}
	if result.IsError {
		// Surface tool-level error text for diagnostics.
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(mcp.TextContent); ok {
				return nil, fmt.Errorf("tool error: %s", tc.Text)
			}
		}
		return nil, fmt.Errorf("tool returned IsError=true")
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("tool returned no content")
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("expected TextContent, got %T", result.Content[0])
	}

	var resp recallResponse
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse recall response: %w", err)
	}

	ids := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		ids = append(ids, r.Memory.ID)
	}
	return ids, nil
}

// buildSummary formats the final evaluation summary block.
func buildSummary(total, k int, avgP, avgMRR, avgNDCG float64, provider, outputFile string) string {
	var sb strings.Builder
	sb.WriteString("--- Evaluation Summary ---\n")
	fmt.Fprintf(&sb, "Queries:      %d\n", total)
	fmt.Fprintf(&sb, "Avg P@%d:      %.2f\n", k, avgP)
	fmt.Fprintf(&sb, "Avg MRR:      %.2f\n", avgMRR)
	fmt.Fprintf(&sb, "Avg NDCG@%d:   %.2f\n", k, avgNDCG)
	fmt.Fprintf(&sb, "Provider:     %s\n", provider)
	if outputFile != "" {
		fmt.Fprintf(&sb, "Written to:   %s\n", outputFile)
	}
	return sb.String()
}

// envOr returns the value of the named env var, or fallback when unset/empty.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
