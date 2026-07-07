// Command wp05-trace compares retrofit recall + Layer B behavior for selected
// LME-S multi-session items against data already ingested via wp05-retrofit-runner.
// Diagnostic only — not part of the scored A/B.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/aggq"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/wp05retrofit"
)

type recallHit struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type itemTrace struct {
	QuestionID       string      `json:"question_id"`
	Question         string      `json:"question"`
	GoldAnswer       string      `json:"gold_answer"`
	Project          string      `json:"project"`
	SessionsIngested int         `json:"sessions_ingested"`
	Anchor           string      `json:"anchor"`
	AggregationQ     bool        `json:"aggregation_expected"`
	RecallCount      int         `json:"recall_count"`
	TopHits          []recallHit `json:"top_hits"`
	LayerBFired      bool        `json:"layer_b_fired"`
	LayerBCount      int         `json:"layer_b_count"`
	LayerBEvidence   int         `json:"layer_b_evidence"`
	GoldInTopK       bool        `json:"gold_literal_in_topk"`
}

func main() {
	var (
		fixturePath   = flag.String("fixture", "/tmp/lme_s_multisession_133.json", "LME-S multi-session fixture")
		idsCSV        = flag.String("ids", "", "comma-separated question_ids to trace (required)")
		projectPrefix = flag.String("project-prefix", "wp05-retrofit-2026-07-04c", "ingested project prefix")
		serverURL     = flag.String("url", "http://127.0.0.1:8790", "Engram MCP server URL")
		apiKey        = flag.String("api-key", "", "Engram API key (env ENGRAM_API_KEY)")
		limit                 = flag.Int("limit", 200, "recall limit")
		exhaustiveAggregation = flag.Bool("exhaustive-aggregation", false, "H8 exhaustive recall path")
		outPath               = flag.String("out", "-", "output JSON path (- for stdout)")
	)
	flag.Parse()

	if strings.TrimSpace(*idsCSV) == "" {
		fmt.Fprintln(os.Stderr, "wp05-trace: -ids is required")
		os.Exit(2)
	}
	if strings.TrimSpace(*apiKey) == "" {
		*apiKey = strings.TrimSpace(os.Getenv("ENGRAM_API_KEY"))
	}
	if *apiKey == "" {
		*apiKey = "wp05-trace-local"
	}

	targets := splitCSV(*idsCSV)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	items, err := wp05retrofit.LoadFixture(*fixturePath)
	if err != nil {
		fatal("load fixture: %v", err)
	}
	byID := make(map[string]wp05retrofit.Item, len(items))
	for _, it := range items {
		byID[it.QuestionID] = it
	}

	client, err := longmemeval.Connect(ctx, *serverURL, *apiKey)
	if err != nil {
		fatal("connect MCP: %v", err)
	}
	defer func() { _ = client.Close() }()

	traces := make([]itemTrace, 0, len(targets))
	for _, id := range targets {
		it, ok := byID[id]
		if !ok {
			fatal("question_id %q not found in fixture", id)
		}
		project := fmt.Sprintf("%s-%s", strings.TrimSuffix(*projectPrefix, "-"), id)
		res, err := wp05retrofit.RecallItem(ctx, client, project, it, wp05retrofit.Config{
			Limit:                 *limit,
			ExhaustiveAggregation: *exhaustiveAggregation,
		})
		if err != nil {
			fatal("recall %s: %v", id, err)
		}

		hits := make([]recallHit, 0, len(res.Results))
		goldInTop := false
		gold := goldString(it.Answer)
		for i, r := range res.Results {
			if r.Memory == nil {
				continue
			}
			preview := strings.ReplaceAll(r.Memory.Content, "\n", " ")
			if len(preview) > 160 {
				preview = preview[:160] + "..."
			}
			hits = append(hits, recallHit{ID: r.Memory.ID, Score: r.Score, Preview: preview})
			if gold != "" && strings.Contains(r.Memory.Content, gold) {
				goldInTop = true
			}
			if i >= 9 {
				continue
			}
		}

		lbCount := 0
		lbEvidence := 0
		lbFired := res.LayerB != nil
		if res.LayerB != nil {
			lbCount = res.LayerB.Count
			lbEvidence = len(res.LayerB.Evidence)
		}

		traces = append(traces, itemTrace{
			QuestionID:       id,
			Question:         it.Question,
			GoldAnswer:       gold,
			Project:          project,
			SessionsIngested: len(it.HaystackSessions),
			Anchor:           aggq.ExtractAggregationAnchor(it.Question),
			AggregationQ:     aggq.IsAggregationQuestion(it.Question),
			RecallCount:      len(res.Results),
			TopHits:          hits,
			LayerBFired:      lbFired,
			LayerBCount:      lbCount,
			LayerBEvidence:   lbEvidence,
			GoldInTopK:       goldInTop,
		})
	}

	payload, err := json.MarshalIndent(map[string]any{
		"system":         "engram-go-retrofit",
		"project_prefix": *projectPrefix,
		"limit":                   *limit,
		"exhaustive_aggregation":  *exhaustiveAggregation,
		"server_url":     *serverURL,
		"traces":         traces,
	}, "", "  ")
	if err != nil {
		fatal("marshal: %v", err)
	}
	if *outPath == "-" {
		fmt.Println(string(payload))
		return
	}
	if err := os.WriteFile(*outPath, payload, 0o644); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
}

func goldString(answer interface{}) string {
	switch v := answer.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wp05-trace: "+format+"\n", args...)
	os.Exit(1)
}
