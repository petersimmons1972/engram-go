package main

// atom_oracle.go — Phase 2A oracle atom-ceiling probe (#1079).
//
// Oracle mode bypasses Engram recall entirely. For each question, atoms are
// extracted locally from the question's gold answer-sessions only, then injected
// directly as the generation context. This isolates whether the atom
// representation is sufficient to carry answers into generation, with retrieval
// noise and extraction-source noise both eliminated.
//
// This is a benchmark-only mode (--atom-oracle flag). OFF by default. Zero
// change to production engram-go recall.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/search"
)

// goldSessionTexts filters HaystackSessions to only those whose corresponding
// HaystackSessionIDs entry appears in item.AnswerSessionIDs. Order is preserved.
func goldSessionTexts(item longmemeval.Item) []string {
	answerSet := make(map[string]bool, len(item.AnswerSessionIDs))
	for _, id := range item.AnswerSessionIDs {
		answerSet[id] = true
	}

	var out []string
	for i, session := range item.HaystackSessions {
		sid := ""
		if i < len(item.HaystackSessionIDs) {
			sid = item.HaystackSessionIDs[i]
		}
		if !answerSet[sid] {
			continue
		}
		var sb strings.Builder
		for _, turn := range session {
			sb.WriteString(turn.Role + ": " + turn.Content + "\n")
		}
		out = append(out, sb.String())
	}
	return out
}

// extractAtomsFromSessions calls atom.NewClaudeExtractor and extracts atoms
// serially from each session text. Serial extraction is correct for oracle
// runs, which operate on a small subset of gold sessions (typically 1–3).
func extractAtomsFromSessions(ctx context.Context, completer atom.ClaudeCompleter, sessionTexts []string) ([]atom.Atom, error) {
	extractor := atom.NewClaudeExtractor(completer)
	var all []atom.Atom
	for _, text := range sessionTexts {
		if text == "" {
			continue
		}
		sessCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		atoms, err := extractor.Extract(sessCtx, text)
		cancel()
		if err != nil {
			// Log but continue: a single bad session must not abort the whole item.
			log.Printf("WARN atom-oracle extract session: %v", err)
			continue
		}
		all = append(all, atoms...)
	}
	return all, nil
}

// oracleGenerateFn is the injectable generation function type for oracle mode.
// It allows tests to replace real OAI generation with a stub.
type oracleGenerateFn func(ctx context.Context, prompt string) (string, error)

// buildOracleContext is the main oracle function. It finds the gold sessions
// from item, extracts atoms from them, and returns formatted context blocks.
//
// Variants:
//   - "atom-only" (default): context blocks = [atom context block]
//   - "atom-plus-source": context blocks = [atom context block, raw session text...]
//
// When zero atoms are extracted, falls back to returning raw gold session text as
// the oracle context (oracle_zero_atoms=true). This preserves oracle measurement
// integrity: the ceiling is "what's achievable with perfect access to gold sessions?"
// Zero-atom items are flagged separately and do not cause an error.
func buildOracleContext(ctx context.Context, completer atom.ClaudeCompleter, cfg *Config, item longmemeval.Item) (contextBlocks []string, atomCount int, sessionCount int, err error) {
	sessionTexts := goldSessionTexts(item)
	if len(sessionTexts) == 0 {
		return nil, 0, 0, fmt.Errorf("oracle: no gold sessions found (answer_session_ids=%v)", item.AnswerSessionIDs)
	}

	atoms, err := extractAtomsFromSessions(ctx, completer, sessionTexts)
	if err != nil {
		return nil, 0, len(sessionTexts), fmt.Errorf("oracle: extraction error: %w", err)
	}

	if len(atoms) == 0 {
		// Graceful fallback: atom extractor found no preference-type atoms (common
		// for non-preference sessions). Rather than excluding the item from scoring,
		// inject raw gold session text as the oracle context.
		// This is still oracle knowledge — the ceiling is "achievable with perfect
		// access to gold sessions?" The caller sets OracleZeroAtoms=true to flag
		// these items in the checkpoint.
		log.Printf("oracle: zero atoms for item (fallback to raw session text), sessions=%d", len(sessionTexts))
		return sessionTexts, 0, len(sessionTexts), nil
	}

	atomBlock := search.FormatAtomsAsContext(atoms)
	contextBlocks = []string{atomBlock}

	if cfg.AtomOracleVariant == "atom-plus-source" {
		contextBlocks = append(contextBlocks, sessionTexts...)
	}

	return contextBlocks, len(atoms), len(sessionTexts), nil
}

// runOneOracleWithDeps handles oracle mode for a single item with injectable
// dependencies (completer and generateFn) for testability. It replaces the
// normal recall+fetch pipeline: atoms are extracted locally from gold sessions
// and injected as the generation context.
func runOneOracleWithDeps(ctx context.Context, cfg *Config, completer atom.ClaudeCompleter, generateFn oracleGenerateFn, item longmemeval.Item, _ longmemeval.IngestEntry) longmemeval.RunEntry {
	contextBlocks, atomCount, sessionCount, err := buildOracleContext(ctx, completer, cfg, item)
	if err != nil {
		log.Printf("WARN oracle [%s] extraction error: %v", item.QuestionID, err)
		return longmemeval.RunEntry{
			QuestionID:      item.QuestionID,
			Status:          "error",
			Error:           err.Error(),
			OracleZeroAtoms: false, // error was "no sessions found", not "extraction returned zero"
		}
	}

	oracleZeroAtoms := atomCount == 0
	log.Printf("oracle [%s] atoms=%d sessions=%d variant=%s zero_atoms=%v", item.QuestionID, atomCount, sessionCount, cfg.AtomOracleVariant, oracleZeroAtoms)

	// Build the generation prompt through the same flag-aware selection path as
	// normal recall. contextBlocks already contains oracle atoms (and optionally
	// raw session text).
	runOpts := longmemeval.RunOpts{
		ExhaustiveAggregation: cfg.ExhaustiveAggregation,
		EnumerateFirst:        cfg.EnumerateFirst,
	}
	prompt := selectGenerationPrompt(cfg, runOpts, item, contextBlocks)

	hypothesis, genErr := generateFn(ctx, prompt)
	if genErr != nil {
		return longmemeval.RunEntry{
			QuestionID:      item.QuestionID,
			Status:          "error",
			Error:           fmt.Sprintf("oracle generate: %v", genErr),
			OracleAtomCount: atomCount,
		}
	}

	return longmemeval.RunEntry{
		QuestionID:      item.QuestionID,
		Hypothesis:      hypothesis,
		RetrievedIDs:    nil, // oracle bypasses Engram recall — no retrieved IDs
		Status:          "done",
		OracleAtomCount: atomCount,
		OracleZeroAtoms: oracleZeroAtoms,
	}
}

// runOneOracle handles oracle mode for a single item. Thin wrapper around
// runOneOracleWithDeps that wires real production dependencies.
func runOneOracle(ctx context.Context, cfg *Config, item longmemeval.Item, ingest longmemeval.IngestEntry) longmemeval.RunEntry {
	completer := &oaiCompleterAdapter{
		baseURL: cfg.LLMBaseURL,
		model:   cfg.LLMModel,
		retries: cfg.Retries,
	}
	opts := longmemeval.OAIOptions{
		EnableThinking: cfg.EnableThinking,
		MaxTokens:      cfg.LLMMaxTokens,
		APIKey:         cfg.LLMApiKey,
	}
	if opts.MaxTokens == 0 && cfg.EnableThinking {
		opts.MaxTokens = 8192
	}
	generateFn := func(ctx context.Context, prompt string) (string, error) {
		return longmemeval.GenerateOAIWithOpts(ctx, prompt, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries, opts)
	}
	return runOneOracleWithDeps(ctx, cfg, completer, generateFn, item, ingest)
}
