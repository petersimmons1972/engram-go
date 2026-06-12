package main

// retrieval_metrics.go — generator-free retrieval metric subcommand (Deliverable A).
//
// Usage:
//
//	longmemeval retrieval-metrics --data <path> --results <dir> [--out <dir>]
//
// Reads checkpoint-ingest.jsonl and checkpoint-run.jsonl from --results,
// joins with the dataset to obtain gold answer_session_ids, and computes
// session-level retrieval metrics WITHOUT calling the generator.
//
// Output: a retrieval_metrics_report.json in --out (default: --results dir),
// and a human-readable table printed to stdout.
//
// Metrics per question and per type:
//   - gold_in_context_rate: fraction where ANY gold session was retrieved
//   - gold_all_rate: fraction where ALL gold sessions were retrieved
//   - recall@5, recall@10: fraction of questions with a gold session in top-K
//   - ndcg@5: mean NDCG@5 over questions
//   - avg_gold_rank: mean rank of first gold session (among found items)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type retrievalMetricsConfig struct {
	DataFile   string
	ResultsDir string
	OutDir     string
}

// runRetrievalMetrics is the entry point for the retrieval-metrics subcommand.
func runRetrievalMetrics(cfg retrievalMetricsConfig, stdout io.Writer) int {
	// Load dataset for gold answer_session_ids and question types.
	items, err := loadItemsFile(cfg.DataFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: %v\n", err)
		return 1
	}
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	// Load ingest checkpoint for memoryMap (memory_id → session_id).
	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(cfg.ResultsDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: read ingest checkpoint: %v\n", err)
		return 1
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}

	// Load run checkpoint for retrieved_ids.
	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.ResultsDir, "checkpoint-run.jsonl"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: read run checkpoint: %v\n", err)
		return 1
	}
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, e := range runEntries {
		if e.Status == "done" {
			runMap[e.QuestionID] = e
		}
	}

	if len(runEntries) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "retrieval-metrics: no done run entries found in checkpoint-run.jsonl")
		return 1
	}

	// Score each question.
	results := make([]longmemeval.ItemRetrievalResult, 0, len(runMap))
	var skipped int
	for qid, run := range runMap {
		item, ok := itemMap[qid]
		if !ok {
			skipped++
			continue
		}
		ingest, ok := ingestMap[qid]
		if !ok {
			skipped++
			continue
		}
		r := longmemeval.ScoreRetrievalForItem(run.RetrievedIDs, ingest.MemoryMap, item.AnswerSessionIDs)
		r.QuestionID = qid
		r.QuestionType = item.QuestionType
		results = append(results, r)
	}

	if skipped > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: skipped %d items (missing in dataset or ingest checkpoint)\n", skipped)
	}
	if len(results) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "retrieval-metrics: no results to report")
		return 1
	}

	report := longmemeval.AggregateRetrievalReport(results)

	// Write JSON report.
	outDir := cfg.OutDir
	if outDir == "" {
		outDir = cfg.ResultsDir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: mkdir %s: %v\n", outDir, err)
		return 1
	}
	reportPath := filepath.Join(outDir, "retrieval_metrics_report.json")
	if err := writeRetrievalReport(reportPath, report); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "retrieval-metrics: write report: %v\n", err)
		return 1
	}

	// Print human-readable table.
	printRetrievalTable(stdout, report, len(results))

	_, _ = fmt.Fprintf(stdout, "\nJSON report written to %s\n", reportPath)
	return 0
}

// printRetrievalTable writes a formatted retrieval metric table to w.
func printRetrievalTable(w io.Writer, report longmemeval.RetrievalReport, total int) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Type\tN\tGoldInCtx%%\tGoldAll%%\tRecall@5\tRecall@10\tNDCG@5\tAvgRank\n")
	_, _ = fmt.Fprintf(tw, "----\t-\t----------\t---------\t--------\t---------\t------\t-------\n")

	// Sort types for deterministic output.
	types := make([]string, 0, len(report.ByType))
	for qt := range report.ByType {
		types = append(types, qt)
	}
	sort.Strings(types)

	for _, qt := range types {
		s := report.ByType[qt]
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%.1f%%\t%.1f%%\t%.3f\t%.3f\t%.3f\t%.1f\n",
			qt, s.N,
			s.GoldInContextRate*100,
			s.GoldAllRate*100,
			s.AvgRecallAt5,
			s.AvgRecallAt10,
			s.AvgNDCGAt5,
			s.AvgGoldRank,
		)
	}
	o := report.Overall
	_, _ = fmt.Fprintf(tw, "OVERALL\t%d\t%.1f%%\t%.1f%%\t%.3f\t%.3f\t%.3f\t%.1f\n",
		o.N,
		o.GoldInContextRate*100,
		o.GoldAllRate*100,
		o.AvgRecallAt5,
		o.AvgRecallAt10,
		o.AvgNDCGAt5,
		o.AvgGoldRank,
	)
	_ = tw.Flush()
}

// writeRetrievalReport marshals the report to a JSON file.
func writeRetrievalReport(path string, report longmemeval.RetrievalReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
