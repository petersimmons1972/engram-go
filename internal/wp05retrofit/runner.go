package wp05retrofit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/petersimmons1972/engram/internal/aggq"
	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/types"
)

const (
	SystemName          = "engram-go-retrofit"
	SubstrateName       = "retrofit"
	SubstrateAssessment = "unknown"
	MeasuredPathLayerB  = "layer_b_only"
)

const constituentMetricsNote = "constituent_recall and constituent_precision are unmeasured at this stage " +
	"(no gold constituent-span annotations are parsed yet); explicit 0.0 floor, not an estimate"

// Item and Turn intentionally reuse the proven LongMemEval fixture schema.
type Item = longmemeval.Item
type Turn = longmemeval.Turn

// Bundle matches duramind/internal/wp05c.Bundle exactly at the JSON level.
type Bundle struct {
	System              string       `json:"system"`
	Substrate           string       `json:"substrate"`
	SubstrateAssessment string       `json:"substrate_assessment"`
	Items               []ItemResult `json:"items"`
}

// ItemResult matches duramind/internal/wp05c.ItemResult exactly at the JSON level.
type ItemResult struct {
	QuestionID           string     `json:"question_id"`
	QuestionType         string     `json:"question_type"`
	Split                string     `json:"split"`
	Variant              string     `json:"variant"`
	AggregationExpected  bool       `json:"aggregation_expected"`
	SolvedType           bool       `json:"solved_type"`
	Fired                bool       `json:"fired"`
	MeasuredPath         string     `json:"measured_path"`
	Confounds            []string   `json:"confounds,omitempty"`
	ConstituentRecall    float64    `json:"constituent_recall"`
	ConstituentPrecision float64    `json:"constituent_precision"`
	DedupAccuracy        float64    `json:"dedup_accuracy"`
	ScopeAccuracy        float64    `json:"scope_accuracy"`
	ArithmeticCorrect    float64    `json:"arithmetic_correctness"`
	Notes                []string   `json:"notes,omitempty"`
	Provenance           Provenance `json:"provenance"`
}

// Provenance matches duramind/internal/wp05c.Provenance exactly at the JSON level.
type Provenance struct {
	GoldVersion   string   `json:"gold_version"`
	ScorerVersion string   `json:"scorer_version"`
	FeatureFlags  []string `json:"feature_flags"`
	System        string   `json:"system"`
	ItemSet       string   `json:"item_set"`
	RunID         string   `json:"run_id"`
	HarnessSHA    string   `json:"harness_sha"`
}

// Client is the narrow harness seam used by the retrofit runner.
type Client interface {
	Store(context.Context, string, string, []string) (string, error)
	StoreBatch(context.Context, string, []longmemeval.BatchItem) ([]string, error)
	RecallFullResult(context.Context, string, string, int) (longmemeval.RecallResult, error)
	ListProjectMemories(context.Context, string, int) ([]types.SearchResult, error)
}

// Config controls one retrofit runner execution.
type Config struct {
	ProjectPrefix         string
	Limit                 int
	ExhaustiveAggregation bool
	ProvenanceTemplate    Provenance
}

// Logf is the minimal logging seam shared by the runner and CLI.
type Logf func(format string, args ...interface{})

// LoadFixture reuses the existing LongMemEval fixture schema and types.
func LoadFixture(path string) ([]Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("fixture %s contains zero items", path)
	}
	return items, nil
}

// Run executes the retrofit measurement for the provided fixture items.
func Run(ctx context.Context, client Client, items []Item, cfg Config, logf Logf) (Bundle, error) {
	if client == nil {
		return Bundle{}, fmt.Errorf("client is nil")
	}
	if len(items) == 0 {
		return Bundle{}, fmt.Errorf("fixture contains zero items")
	}
	if strings.TrimSpace(cfg.ProjectPrefix) == "" {
		return Bundle{}, fmt.Errorf("project prefix is empty")
	}
	if cfg.Limit <= 0 {
		return Bundle{}, fmt.Errorf("limit must be > 0")
	}

	results := make([]ItemResult, 0, len(items))
	for _, item := range items {
		project := cfg.ProjectPrefix + "-" + item.QuestionID
		if err := IngestItem(ctx, client, project, item); err != nil {
			return Bundle{}, fmt.Errorf("ingest item %s: %w", item.QuestionID, err)
		}
		recallResult, err := RecallItem(ctx, client, project, item, cfg)
		if err != nil {
			return Bundle{}, fmt.Errorf("recall item %s: %w", item.QuestionID, err)
		}
		scored := ScoreItem(item, recallResult, cfg.ProvenanceTemplate)
		results = append(results, scored)
		if logf != nil {
			logf("wp05-retrofit: item=%s sessions=%d fired=%t solved_type=%t",
				item.QuestionID, len(item.HaystackSessions), scored.Fired, scored.SolvedType)
		}
	}

	return Bundle{
		System:              SystemName,
		Substrate:           SubstrateName,
		SubstrateAssessment: SubstrateAssessment,
		Items:               results,
	}, nil
}

// IngestItem writes one memory per haystack session under the given project.
func IngestItem(ctx context.Context, client Client, project string, item Item) error {
	if len(item.HaystackSessions) == 0 {
		return fmt.Errorf("item %s has zero haystack sessions", item.QuestionID)
	}
	if len(item.HaystackSessions) == 1 {
		_, err := client.Store(ctx, project, renderSessionContent(sessionDate(item, 0), item.HaystackSessions[0]), sessionTags(item, 0))
		if err != nil {
			return fmt.Errorf("store session 0: %w", err)
		}
		return nil
	}

	batch := make([]longmemeval.BatchItem, 0, len(item.HaystackSessions))
	for i, turns := range item.HaystackSessions {
		batch = append(batch, longmemeval.BatchItem{
			Content: renderSessionContent(sessionDate(item, i), turns),
			Tags:    sessionTags(item, i),
		})
	}
	if _, err := client.StoreBatch(ctx, project, batch); err != nil {
		return fmt.Errorf("store batch: %w", err)
	}
	return nil
}

func renderSessionContent(date string, turns []Turn) string {
	var b strings.Builder
	b.WriteString("Session date: ")
	b.WriteString(date)
	b.WriteString("\n")
	for _, turn := range turns {
		b.WriteString(turn.Role)
		b.WriteString(": ")
		b.WriteString(turn.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func sessionDate(item Item, idx int) string {
	if idx >= 0 && idx < len(item.HaystackDates) {
		return item.HaystackDates[idx]
	}
	return ""
}

func sessionTags(item Item, idx int) []string {
	if idx >= 0 && idx < len(item.HaystackSessionIDs) && strings.TrimSpace(item.HaystackSessionIDs[idx]) != "" {
		return []string{"session:" + strings.TrimSpace(item.HaystackSessionIDs[idx])}
	}
	return nil
}

// RecallItem runs the full-mode recall path so additive layer_b data survives transport.
// When cfg.ExhaustiveAggregation is set and the question is aggregation-shaped, H8
// deep recall plus anchor and project sweeps are unioned and Layer B is built
// client-side over the merged candidate set.
func RecallItem(ctx context.Context, client Client, project string, item Item, cfg Config) (longmemeval.RecallResult, error) {
	if cfg.ExhaustiveAggregation && longmemeval.IsAggregationQuestion(item.Question) {
		return recallItemExhaustive(ctx, client, project, item, cfg.Limit)
	}
	return client.RecallFullResult(ctx, project, item.Question, cfg.Limit)
}

// ScoreItem ports duramind's wp05b scorer semantics to the retrofit harness.
func ScoreItem(item Item, recallResult longmemeval.RecallResult, provenance Provenance) ItemResult {
	fired := recallResult.LayerB != nil
	aggregationExpected := aggq.IsAggregationQuestion(item.Question)

	goldAnswer, goldOK := ParseIntAnswer(string(item.Answer))

	solvedType := false
	arithmeticCorrect := 0.0
	notes := make([]string, 0, 2)
	switch {
	case !goldOK:
		notes = append(notes, "gold answer did not parse as a clean integer; arithmetic_correctness forced to 0.0 (not estimated)")
	case fired && recallResult.LayerB.Count == goldAnswer:
		solvedType = true
		arithmeticCorrect = 1.0
	}
	notes = append(notes, constituentMetricsNote)

	return ItemResult{
		QuestionID:          item.QuestionID,
		QuestionType:        item.QuestionType,
		Split:               "develop",
		Variant:             "base",
		AggregationExpected: aggregationExpected,
		SolvedType:          solvedType,
		Fired:               fired,
		// tools_recall.go computes layer_b additively on the non-handle path:
		// the base recall results stay intact and layer_b is attached beside them.
		MeasuredPath:         MeasuredPathLayerB,
		ConstituentRecall:    0.0,
		ConstituentPrecision: 0.0,
		DedupAccuracy:        dedupAccuracy(recallResult.LayerB),
		// Scope accuracy is guaranteed by per-item project isolation.
		ScopeAccuracy:     1.0,
		ArithmeticCorrect: arithmeticCorrect,
		Notes:             notes,
		Provenance:        provenance,
	}
}

// ParseIntAnswer ports the greenfield runner's "clean integer only" parsing.
func ParseIntAnswer(answer any) (value int, ok bool) {
	switch v := answer.(type) {
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		s := strings.TrimSpace(v)
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func dedupAccuracy(summary *layerb.Summary) float64 {
	if summary == nil || len(summary.Evidence) == 0 {
		return 1.0
	}
	seen := make(map[string]int, len(summary.Evidence))
	for _, ev := range summary.Evidence {
		seen[ev.ProvenanceSpan]++
	}
	dupCount := 0
	for _, count := range seen {
		if count > 1 {
			dupCount += count - 1
		}
	}
	return 1.0 - float64(dupCount)/float64(len(summary.Evidence))
}

// WriteBundle writes the bundle as stable, human-readable JSON.
func WriteBundle(path string, bundle Bundle) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bundle: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
