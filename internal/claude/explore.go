// Package claude: iterative RLM-style synthesis (memory_explore).
//
// Explore runs a bounded recall → score → refine loop server-side, collapsing
// N client round-trips into a single synchronous call. The client sees only
// the final synthesized answer plus grounding metadata.

package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// Recaller retrieves memories by semantic query. Declared here to avoid a
// circular import on the search package.
type Recaller interface {
	Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error)
}

// RelationshipGetter fetches relationships for conflict detection. Declared
// here to avoid a circular import on the db package.
type RelationshipGetter interface {
	GetRelationships(ctx context.Context, project, memoryID string) ([]types.Relationship, error)
}

// MemoryFetcher retrieves a single memory by ID at full detail.
// Passed to Explore to upgrade corpus entries to full content before synthesis.
// Implementations may ignore the project parameter if the backend is project-agnostic.
type MemoryFetcher interface {
	FetchMemory(ctx context.Context, project, memoryID string) (*types.Memory, error)
}

// ExploreScope constrains which memories are eligible for recall during Explore.
// All non-zero fields are applied as AND conditions. Applied by the handler via
// a scopedRecaller wrapper — Explore itself does not filter.
type ExploreScope struct {
	Tags      []string   `json:"tags,omitempty"`
	EpisodeID string     `json:"episode_id,omitempty"`
	Since     *time.Time `json:"since,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
}

// ExploreRequest is the input envelope for Explore.
type ExploreRequest struct {
	Project             string
	Question            string
	MaxIterations       int
	ConfidenceThreshold float64
	TokenBudget         int
	IncludeTrace        bool
	Scope               ExploreScope
}

// TraceStep records one iteration of the explore loop for debugging.
type TraceStep struct {
	Iteration    int      `json:"iteration"`
	Query        string   `json:"query"`
	NewMemoryIDs []string `json:"new_memory_ids"`
	Confidence   float64  `json:"confidence"`
	Gaps         []string `json:"gaps"`
	RefinedQuery string   `json:"refined_query,omitempty"`
	Stop         bool     `json:"stop"`
	TokensUsed   int      `json:"tokens_used"`
}

// ExploreResult is the output envelope from Explore.
type ExploreResult struct {
	Answer     string         `json:"answer"`
	MemoryIDs  []string       `json:"memory_ids"`
	Iterations int            `json:"iterations"`
	Confidence float64        `json:"confidence"`
	Conflicts  []ConflictPair `json:"conflicts"`
	Truncated  bool           `json:"truncated,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	Trace      []TraceStep    `json:"trace,omitempty"`
}

const exploreScoringSystem = `You are a memory recall quality scorer. Evaluate the retrieved memory corpus against the question on exactly three criteria:
1. Direct answer: Do the retrieved memories directly answer the question?
2. Contradictions: Are there unresolved contradictions among the memories?
3. Missing entities: Are there named entities in the question or memories that have NOT been retrieved?

Respond with valid JSON only (no markdown fences):
{"confidence": <0.0-1.0>, "gaps": [<string>], "refined_query": <string or null>, "stop": <bool>}
- confidence: 0.0=no answer, 1.0=fully answered
- gaps: list of specific missing entities or information
- refined_query: a better search query to fill gaps, or null if none needed
- stop: true if corpus has enough information to answer the question`

// scoringResponse is what the sub-Claude scoring call should produce.
type scoringResponse struct {
	Confidence   float64  `json:"confidence"`
	Gaps         []string `json:"gaps"`
	RefinedQuery *string  `json:"refined_query"`
	Stop         bool     `json:"stop"`
}

// exploreSynthThreshold is the corpus size above which Explore fans out to
// sharded synthesis. Below this, it goes straight to conflict-aware reasoning.
const exploreSynthThreshold = 20

// budgetTracker tracks cumulative token usage and flags exhaustion.
type budgetTracker struct {
	used   int
	budget int
}

func (b *budgetTracker) add(u TokenUsage) { b.used += u.Total() }
func (b *budgetTracker) exhausted() bool  { return b.budget > 0 && b.used >= b.budget }

// Explore runs the iterative recall+score+synthesis loop.
//
// Contract:
//   - Stops when confidence >= threshold && gaps empty, or iter >= max_iterations,
//     or tokens >= budget (sets Truncated=true), or refined_query == previous,
//     or two consecutive zero-new-memory iterations.
//   - If fetcher is non-nil, corpus entries are upgraded to full-detail content
//     after the loop completes and before synthesis (best-effort: errors keep
//     the summary-level content).
//   - Returns a grounded answer; ungrounded UUID-like citations are stripped and
//     reported as warnings.
func Explore(ctx context.Context, c *Client, r Recaller, fetcher MemoryFetcher, rels RelationshipGetter, req ExploreRequest) (*ExploreResult, error) {
	if c == nil {
		return nil, fmt.Errorf("explore: claude client is nil")
	}
	if r == nil {
		return nil, fmt.Errorf("explore: recaller is nil")
	}
	if strings.TrimSpace(req.Question) == "" {
		return nil, fmt.Errorf("explore: question is required")
	}
	maxIter := req.MaxIterations
	if maxIter < 1 {
		maxIter = 5
	}
	if maxIter > 10 {
		maxIter = 10
	}
	threshold := req.ConfidenceThreshold
	if threshold <= 0 {
		threshold = 0.75
	}
	budget := req.TokenBudget
	if budget <= 0 {
		budget = 20000
	}

	result := &ExploreResult{}
	tracker := &budgetTracker{budget: budget}

	// Ordered corpus (ID → *Memory) so we preserve insertion order.
	corpus := make(map[string]*corpusEntry)
	var corpusOrder []string
	corpusIDs := make(map[string]bool)

	query := req.Question
	prevQuery := ""
	zeroNewStreak := 0
	var lastConfidence float64
	truncated := false

	iter := 0
	for iter < maxIter {
		if tracker.exhausted() {
			truncated = true
			break
		}

		// 1. Recall.
		results, err := r.Recall(ctx, query, 15, "summary")
		if err != nil {
			return nil, fmt.Errorf("explore: recall iter %d: %w", iter, err)
		}

		// 2. Filter already-seen IDs.
		var newIDs []string
		for _, sr := range results {
			if sr.Memory == nil {
				continue
			}
			id := sr.Memory.ID
			if _, seen := corpus[id]; seen {
				continue
			}
			corpus[id] = &corpusEntry{mem: sr.Memory}
			corpusOrder = append(corpusOrder, id)
			corpusIDs[id] = true
			newIDs = append(newIDs, id)
		}

		// 3. Score.
		score, usage, scoreErr := scoreCorpus(ctx, c, req.Question, corpus, corpusOrder)
		tracker.add(usage)

		step := TraceStep{
			Iteration:    iter,
			Query:        query,
			NewMemoryIDs: newIDs,
			TokensUsed:   usage.Total(),
		}
		if scoreErr == nil {
			step.Confidence = score.Confidence
			step.Gaps = score.Gaps
			if score.RefinedQuery != nil {
				step.RefinedQuery = *score.RefinedQuery
			}
			step.Stop = score.Stop
			lastConfidence = score.Confidence
		}
		result.Trace = append(result.Trace, step)

		iter++

		if scoreErr != nil {
			// Scoring failed — treat as low confidence, keep going unless other
			// stop condition triggers.
			if iter >= maxIter {
				break
			}
			if tracker.exhausted() {
				truncated = true
				break
			}
			continue
		}

		// 4. Stop conditions.
		// Budget exhausted.
		if tracker.exhausted() {
			truncated = true
			break
		}
		// Confidence high enough + no gaps.
		if score.Confidence >= threshold && len(score.Gaps) == 0 {
			break
		}
		// Explicit stop flag.
		if score.Stop {
			break
		}
		// Zero-new-memory streak (two in a row).
		if len(newIDs) == 0 {
			zeroNewStreak++
			if zeroNewStreak >= 2 {
				break
			}
		} else {
			zeroNewStreak = 0
		}
		// No refined query → nothing to try next.
		if score.RefinedQuery == nil || *score.RefinedQuery == "" {
			break
		}
		// Refined query equals previous query → no progress.
		if *score.RefinedQuery == prevQuery || *score.RefinedQuery == query {
			break
		}
		prevQuery = query
		query = *score.RefinedQuery
	}

	result.Iterations = iter
	result.Truncated = truncated

	// Final synthesis: upgrade corpus entries to full-detail content so the
	// synthesis call has the richest possible evidence. Best-effort: on any
	// fetch error the summary-level memory is retained.
	if fetcher != nil {
		for _, id := range corpusOrder {
			full, err := fetcher.FetchMemory(ctx, req.Project, id)
			if err == nil && full != nil {
				corpus[id] = &corpusEntry{mem: full}
			}
		}
	}

	// Build ordered memory list.
	memories := make([]*types.Memory, 0, len(corpusOrder))
	for _, id := range corpusOrder {
		memories = append(memories, corpus[id].mem)
	}

	// Build evidence map for conflicts (best-effort).
	var allRels []types.Relationship
	if rels != nil {
		seen := make(map[string]bool)
		for _, m := range memories {
			rs, err := rels.GetRelationships(ctx, req.Project, m.ID)
			if err != nil {
				continue
			}
			for _, rel := range rs {
				if !seen[rel.ID] {
					seen[rel.ID] = true
					allRels = append(allRels, rel)
				}
			}
		}
	}
	ev := DiagnoseMemories(memories, allRels)
	result.Conflicts = ev.Conflicts
	if len(allRels) > 0 {
		result.Confidence = ev.Confidence
	} else {
		result.Confidence = lastConfidence
	}

	// Synthesis.
	var answer string
	var synthErr error
	if len(memories) == 0 {
		answer, synthErr = c.ReasonOverMemories(ctx, req.Question, memories)
	} else if len(memories) > exploreSynthThreshold {
		answer, synthErr = FanOutReason(ctx, c, req.Question, memories, 8, 15)
	} else {
		answer, synthErr = c.ReasonWithConflictAwareness(ctx, req.Question, ev)
	}
	if synthErr != nil {
		return nil, fmt.Errorf("explore: synthesis: %w", synthErr)
	}

	// 6. Citation guard.
	cleanAnswer, warnings := validateCitations(answer, corpusIDs)
	result.Answer = cleanAnswer
	result.Warnings = append(result.Warnings, warnings...)

	// Build the memory_ids list.
	ids := make([]string, 0, len(memories))
	for _, m := range memories {
		ids = append(ids, m.ID)
	}
	result.MemoryIDs = ids

	if !req.IncludeTrace {
		result.Trace = nil
	}

	return result, nil
}

// scoreCorpus runs one scoring sub-Claude call.
func scoreCorpus(ctx context.Context, c *Client, question string, corpus map[string]*corpusEntry, order []string) (scoringResponse, TokenUsage, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Question: %s\n\n", question)
	if len(order) == 0 {
		sb.WriteString("(No memories retrieved yet.)\n")
	} else {
		sb.WriteString("Retrieved memories:\n")
		for i, id := range order {
			m := corpus[id].mem
			content := m.Content
			if len(content) > 400 {
				content = content[:400]
			}
			fmt.Fprintf(&sb, "[%d] ID:%s %s\n", i+1, id, content)
		}
	}
	sb.WriteString("\nEmit ONLY the scoring JSON object.")

	text, usage, err := c.CompleteWithUsage(ctx, exploreScoringSystem, sb.String(), "claude-sonnet-4-6", "claude-opus-4-6", 1, 512)
	if err != nil {
		return scoringResponse{}, usage, err
	}
	var out scoringResponse
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return scoringResponse{}, usage, fmt.Errorf("parse scoring JSON: %w", err)
	}
	return out, usage, nil
}

// corpusEntry wraps a memory in the explore corpus. Single-field struct so
// future fields (e.g. score breakdown) can be added without refactoring.
type corpusEntry struct {
	mem *types.Memory
}

// citationPattern matches bare 32-char lowercase-hex identifiers on word
// boundaries.
var citationPattern = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

// validateCitations scans answer for 32-char hex strings not in corpusIDs,
// strips them, and returns warnings.
func validateCitations(answer string, corpusIDs map[string]bool) (string, []string) {
	var warnings []string
	stripped := false
	cleaned := citationPattern.ReplaceAllStringFunc(answer, func(match string) string {
		if corpusIDs[match] {
			return match
		}
		stripped = true
		return ""
	})
	if stripped {
		// Collapse runs of whitespace introduced by the strip.
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		warnings = append(warnings, "ungrounded_citation_stripped")
	}
	return cleaned, warnings
}
