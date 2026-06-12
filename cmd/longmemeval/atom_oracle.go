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

// buildOracleContext is the main oracle function. It finds the gold sessions
// from item, extracts atoms from them, and returns formatted context blocks.
//
// Variants:
//   - "atom-only" (default): context blocks = [atom context block]
//   - "atom-plus-source": context blocks = [atom context block, raw session text...]
//
// Returns a non-nil error when zero atoms are extracted (fail-closed). A zero-
// atom result would silently score the item as memory-less, hiding the failure.
func buildOracleContext(ctx context.Context, cfg *Config, item longmemeval.Item) (contextBlocks []string, atomCount int, err error) {
	sessionTexts := goldSessionTexts(item)
	if len(sessionTexts) == 0 {
		return nil, 0, fmt.Errorf("oracle: no gold sessions found (answer_session_ids=%v)", item.AnswerSessionIDs)
	}

	completer := &oaiCompleterAdapter{
		baseURL: cfg.LLMBaseURL,
		model:   cfg.LLMModel,
		retries: cfg.Retries,
	}

	atoms, err := extractAtomsFromSessions(ctx, completer, sessionTexts)
	if err != nil {
		return nil, 0, fmt.Errorf("oracle: extraction error: %w", err)
	}
	if len(atoms) == 0 {
		return nil, 0, fmt.Errorf("oracle: zero atoms extracted from %d gold session(s)", len(sessionTexts))
	}

	atomBlock := search.FormatAtomsAsContext(atoms)
	contextBlocks = []string{atomBlock}

	if cfg.AtomOracleVariant == "atom-plus-source" {
		contextBlocks = append(contextBlocks, sessionTexts...)
	}

	return contextBlocks, len(atoms), nil
}

// runOneOracle handles oracle mode for a single item. It replaces the normal
// recall+fetch pipeline entirely: atoms are extracted locally from gold sessions
// and injected as the generation context. The generator is held fixed.
func runOneOracle(ctx context.Context, cfg *Config, item longmemeval.Item, ingest longmemeval.IngestEntry) longmemeval.RunEntry {
	contextBlocks, atomCount, err := buildOracleContext(ctx, cfg, item)
	if err != nil {
		log.Printf("WARN oracle [%s] zero atoms: %v", item.QuestionID, err)
		return longmemeval.RunEntry{
			QuestionID:      item.QuestionID,
			Status:          "error",
			Error:           err.Error(),
			OracleZeroAtoms: true,
		}
	}

	log.Printf("oracle [%s] atoms=%d sessions=%d variant=%s", item.QuestionID, atomCount, len(goldSessionTexts(item)), cfg.AtomOracleVariant)

	// Build generation prompt using the same prompt-assembly path as normal mode.
	// contextBlocks already contains oracle atoms (and optionally raw session text).
	prompt := longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, contextBlocks)

	opts := longmemeval.OAIOptions{
		EnableThinking: cfg.EnableThinking,
		MaxTokens:      cfg.LLMMaxTokens,
		APIKey:         cfg.LLMApiKey,
	}
	if opts.MaxTokens == 0 && cfg.EnableThinking {
		opts.MaxTokens = 8192
	}

	hypothesis, genErr := longmemeval.GenerateOAIWithOpts(ctx, prompt, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries, opts)
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
	}
}
