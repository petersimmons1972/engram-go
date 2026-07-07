// Command wp05-layerb-gate-trace diagnoses which Layer B v4 gate blocks
// BuildSummary for recalled candidate sets. Diagnostic only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/wp05retrofit"
)

type gateTrace struct {
	QuestionID       string            `json:"question_id"`
	Question         string            `json:"question"`
	Project          string            `json:"project"`
	GreenfieldProject string           `json:"greenfield_project,omitempty"`
	SessionsIngested int              `json:"sessions_ingested"`
	RecallCount      int              `json:"recall_count"`
	GreenfieldMemoryCount int         `json:"greenfield_memory_count,omitempty"`
	Exhaustive       bool             `json:"exhaustive_aggregation"`
	BaselineGate     layerb.Diagnosis `json:"baseline_gate"`
	ExhaustiveGate   *layerb.Diagnosis `json:"exhaustive_gate,omitempty"`
	GreenfieldGate   *layerb.Diagnosis `json:"greenfield_gate,omitempty"`
	RetrofitDBGate   *layerb.Diagnosis `json:"retrofit_db_gate,omitempty"`
	RetrofitDBMemoryCount int         `json:"retrofit_db_memory_count,omitempty"`
	LayerBFired      bool             `json:"layer_b_fired"`
	GreenfieldWouldFire bool          `json:"greenfield_would_fire,omitempty"`
	RetrofitDBWouldFire bool          `json:"retrofit_db_would_fire,omitempty"`
}

func main() {
	var (
		fixturePath   = flag.String("fixture", "/tmp/lme_s_multisession_133.json", "LME-S multi-session fixture")
		idsCSV        = flag.String("ids", "", "comma-separated question_ids (required)")
		projectPrefix = flag.String("project-prefix", "wp05-retrofit-2026-07-04c", "ingested project prefix")
		serverURL     = flag.String("url", "http://127.0.0.1:8790", "Engram MCP server URL")
		apiKey        = flag.String("api-key", "", "Engram API key")
		limit         = flag.Int("limit", 200, "recall limit for baseline path")
		exhaustive        = flag.Bool("exhaustive-aggregation", true, "also run exhaustive recall path and diagnose")
		greenfieldDSN     = flag.String("greenfield-dsn", "", "optional Postgres DSN to diagnose greenfield raw_memories (e.g. engram_ng)")
		greenfieldPrefix  = flag.String("greenfield-prefix", "wp05b-refire2-2026-07-04", "greenfield project prefix")
		retrofitDSN       = flag.String("retrofit-dsn", "", "optional Postgres DSN to diagnose retrofit memories directly (e.g. engram_go_retrofit)")
		outPath           = flag.String("out", "-", "output JSON path (- for stdout)")
	)
	flag.Parse()
	if strings.TrimSpace(*idsCSV) == "" {
		fmt.Fprintln(os.Stderr, "wp05-layerb-gate-trace: -ids is required")
		os.Exit(2)
	}
	if strings.TrimSpace(*apiKey) == "" {
		*apiKey = strings.TrimSpace(os.Getenv("ENGRAM_API_KEY"))
	}
	if *apiKey == "" {
		*apiKey = "wp05-local"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	traces := make([]gateTrace, 0)
	for _, id := range splitCSV(*idsCSV) {
		it, ok := byID[id]
		if !ok {
			fatal("question_id %q not in fixture", id)
		}
		project := fmt.Sprintf("%s-%s", strings.TrimSuffix(*projectPrefix, "-"), id)

		baseline, err := wp05retrofit.RecallItem(ctx, client, project, it, wp05retrofit.Config{Limit: *limit})
		if err != nil {
			fatal("baseline recall %s: %v", id, err)
		}
		baseDiag := layerb.DiagnoseBuildSummary(it.Question, baseline.Results)

		trace := gateTrace{
			QuestionID:       id,
			Question:         it.Question,
			Project:          project,
			SessionsIngested: len(it.HaystackSessions),
			RecallCount:      len(baseline.Results),
			Exhaustive:       *exhaustive,
			BaselineGate:     baseDiag,
			LayerBFired:      baseline.LayerB != nil,
		}

		if *exhaustive {
			exh, err := wp05retrofit.RecallItem(ctx, client, project, it, wp05retrofit.Config{
				Limit:                 *limit,
				ExhaustiveAggregation: true,
			})
			if err != nil {
				fatal("exhaustive recall %s: %v", id, err)
			}
			exhDiag := layerb.DiagnoseBuildSummary(it.Question, exh.Results)
			trace.ExhaustiveGate = &exhDiag
			trace.RecallCount = len(exh.Results)
			trace.LayerBFired = exh.LayerB != nil
		}

		if strings.TrimSpace(*greenfieldDSN) != "" {
			gfProject := fmt.Sprintf("%s-%s", strings.TrimSuffix(*greenfieldPrefix, "-"), id)
			trace.GreenfieldProject = gfProject
			gfMemories, err := wp05retrofit.LoadGreenfieldMemories(ctx, *greenfieldDSN, gfProject)
			if err != nil {
				fatal("greenfield load %s: %v", id, err)
			}
			trace.GreenfieldMemoryCount = len(gfMemories)
			gfDiag := layerb.DiagnoseBuildSummary(it.Question, gfMemories)
			trace.GreenfieldGate = &gfDiag
			trace.GreenfieldWouldFire = gfDiag.WouldFire
		}

		if strings.TrimSpace(*retrofitDSN) != "" {
			rfMemories, err := wp05retrofit.LoadRetrofitMemories(ctx, *retrofitDSN, project)
			if err != nil {
				fatal("retrofit db load %s: %v", id, err)
			}
			trace.RetrofitDBMemoryCount = len(rfMemories)
			rfDiag := layerb.DiagnoseBuildSummary(it.Question, rfMemories)
			trace.RetrofitDBGate = &rfDiag
			trace.RetrofitDBWouldFire = rfDiag.WouldFire
		}

		traces = append(traces, trace)
	}

	payload, err := json.MarshalIndent(map[string]any{
		"system":            "engram-go-retrofit",
		"project_prefix":    *projectPrefix,
		"greenfield_prefix":   strings.TrimSpace(*greenfieldPrefix),
		"greenfield_dsn_set": strings.TrimSpace(*greenfieldDSN) != "",
		"retrofit_dsn_set":   strings.TrimSpace(*retrofitDSN) != "",
		"exhaustive":        *exhaustive,
		"traces":            traces,
	}, "", "  ")
	if err != nil {
		fatal("marshal: %v", err)
	}
	if *outPath == "-" {
		fmt.Println(string(payload))
		return
	}
	if err := os.MkdirAll(dirOf(*outPath), 0o755); err != nil {
		fatal("mkdir: %v", err)
	}
	if err := os.WriteFile(*outPath, payload, 0o644); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
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
	fmt.Fprintf(os.Stderr, "wp05-layerb-gate-trace: "+format+"\n", args...)
	os.Exit(1)
}