// instinct-audit-go queries Engram for instinct-detected patterns and evaluates
// each one using a configured LLM backend. Outputs a JSON report to stdout.
//
// Usage: instinct-audit-go [flags]
//
//	-llm-backend string  LLM backend: anthropic or olla (default from LLM_BACKEND env, fallback "anthropic")
//	-timeout dur         Per-inference timeout (default 60s)
//	-engram string       Engram base URL (overrides config)
//	-token string        Engram Bearer token (overrides config)
package main

import "time"

// engramMemory is one record returned by the Engram /quick-recall endpoint.
// Field names match the JSON schema Engram stores.
type engramMemory struct {
	ID         string   `json:"id"`
	Content    string   `json:"content"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags"`
	Importance float64  `json:"importance"`
}

// auditResult holds the per-pattern LLM verdict for one Engram record.
type auditResult struct {
	ID            string   `json:"id"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	Confidence    float64  `json:"confidence"`
	IsValid       string   `json:"is_valid"`
	IsActionable  string   `json:"is_actionable"`
	IsSpecific    string   `json:"is_specific"`
	FalsePositive string   `json:"false_positive"`
	Verdict       string   `json:"verdict"`
	Reason        string   `json:"reason"`
	Raw           string   `json:"raw,omitempty"`
}

// report is the top-level JSON object written to stdout.
//
// CRITICAL: DO NOT rename ANY field. The json:"..." tags total, keep, tune,
// reject, false_positive_rate are the contract with instinct-weekly-audit.sh.
// The downstream consumer is a jq filter — field name typos silently zero
// the weekly metrics.
type report struct {
	Total             int           `json:"total"`
	Keep              int           `json:"keep"`
	Tune              int           `json:"tune"`
	Reject            int           `json:"reject"`
	FalsePositiveRate float64       `json:"false_positive_rate"`
	Patterns          []auditResult `json:"patterns"`
}

// defaultTimeout is the per-inference timeout used when -timeout is not set.
const defaultTimeout = 60 * time.Second
