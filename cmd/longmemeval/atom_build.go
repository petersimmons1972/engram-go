package main

// atom_build.go — "atom-build" subcommand for LME Milestone 1 (#938).
//
// Iterates sessions in the ingest checkpoint, runs preference-atom extraction
// via the local olla OAI endpoint (NOT the claude CLI), embeds each atom's
// statement, and stores both the atom and its embedding via the Engram REST API.
//
// The subcommand is intentionally minimal: it is a one-shot batch processor
// designed to be run after `ingest` and before `run --atom-mode`.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// AtomBuildConfig holds flags for the atom-build subcommand.
type AtomBuildConfig struct {
	DataFile     string
	OutDir       string
	RunID        string
	Workers      int
	ServerURL    string
	APIKey       string
	LLMBaseURL   string
	LLMModel     string
	EmbedURL     string
	EmbedModel   string
	Retries      int
	DatabaseURL  string // when set, store atoms directly via DB connection (--direct-db path)
	AtomCacheDir string // when set, also write atoms to <dir>/<project>.json for local fallback
	Extractor    string // "olla" (default, GenerateOAI) or "sonnet" (claude --print --model sonnet)
	MaxSessions  int    // cap sessions extracted per question (0 = all); answer sessions always included
}

// oaiCompleterAdapter wraps GenerateOAI to satisfy atom.ClaudeCompleter.
// The atom extractor calls Complete(ctx, system, user, ...) — we concatenate
// system + user into a single prompt and call the OAI endpoint.
type oaiCompleterAdapter struct {
	baseURL string
	model   string
	retries int
}

func (a *oaiCompleterAdapter) Complete(ctx context.Context, system, prompt, _, _ string, _ int, maxTokens int) (string, error) {
	// Pass system as the OAI system message, not concatenated into the user prompt.
	// Concatenation was silently overridden by buildOAIRequestBody's fixed default system message,
	// causing the model to ignore the extraction instructions entirely (#1079).
	opts := longmemeval.OAIOptions{
		SystemPrompt: system,
		MaxTokens:    maxTokens,
	}
	return longmemeval.GenerateOAIWithOpts(ctx, prompt, a.baseURL, a.model, a.retries, opts)
}

// sonnetCompleterAdapter satisfies atom.ClaudeCompleter by calling
// `claude --print --model sonnet` (the GenerateSonnet path). Used for the
// best-case extraction arm (founder-authorized Sonnet extraction). The atom
// extractor passes system + prompt; we concatenate them for the single-prompt
// claude --print interface.
type sonnetCompleterAdapter struct {
	retries int
}

func (a *sonnetCompleterAdapter) Complete(ctx context.Context, system, prompt, _, _ string, _ int, _ int) (string, error) {
	combined := system + "\n\n" + prompt
	return longmemeval.GenerateSonnet(ctx, combined, a.retries)
}

// atomBuildEntry is written to checkpoint-atom-build.jsonl.
type atomBuildEntry struct {
	QuestionID string `json:"question_id"`
	Project    string `json:"project"`
	AtomsFound int    `json:"atoms_found"`
	AtomsOK    int    `json:"atoms_ok"`
	Status     string `json:"status"` // "done" | "error"
	Error      string `json:"error,omitempty"`
}

// runAtomBuild is the entry point for the atom-build subcommand.
func runAtomBuild(cfg *AtomBuildConfig, stdout, stderr io.Writer) int {
	if cfg.DataFile == "" {
		_, _ = fmt.Fprintln(stderr, "--data is required")
		return 1
	}
	// --llm-url / --llm-model are only required for the olla extractor.
	// The sonnet extractor uses claude --print and needs neither.
	if cfg.Extractor != "sonnet" {
		if cfg.LLMBaseURL == "" {
			_, _ = fmt.Fprintln(stderr, "--llm-url is required (local olla endpoint) unless --extractor sonnet")
			return 1
		}
		if cfg.LLMModel == "" {
			_, _ = fmt.Fprintln(stderr, "--llm-model is required unless --extractor sonnet")
			return 1
		}
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = "BAAI/bge-m3"
	}
	// Embedding always needs a real endpoint (Claude has no embedding API).
	if cfg.EmbedURL == "" {
		_, _ = fmt.Fprintln(stderr, "--embed-url is required (the embeddings endpoint; it is NOT the same server as --llm-url).")
		_, _ = fmt.Fprintln(stderr, "Pass the base WITHOUT a trailing /v1 — the client appends /v1/embeddings itself.")
		return 1
	}

	// Load ingest checkpoint to know which (project, question_id) pairs exist.
	ingestPath := filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl")
	ingested, err := longmemeval.ReadAllIngest(ingestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "read ingest checkpoint: %v\n", err)
		return 1
	}
	if len(ingested) == 0 {
		_, _ = fmt.Fprintln(stderr, "no ingested sessions found — run 'ingest' first")
		return 1
	}

	// Build skip set from existing atom-build checkpoint.
	buildPath := filepath.Join(cfg.OutDir, "checkpoint-atom-build.jsonl")
	skip, err := readAtomBuildSkipSet(buildPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "read atom-build checkpoint: %v\n", err)
		return 1
	}
	log.Printf("atom-build: %d ingested sessions, %d already done", len(ingested), len(skip))

	// Load dataset for session text.
	items := loadItems(cfg.DataFile)
	itemByID := make(map[string]longmemeval.Item, len(items))
	for _, it := range items {
		itemByID[it.QuestionID] = it
	}

	// Build embedding client (olla OAI-compatible /v1/embeddings).
	embedClient := embed.NewLiteLLMClientNoProbe(cfg.EmbedURL, cfg.EmbedModel, "", 1024)

	// Build the extraction completer. "sonnet" uses claude --print --model sonnet
	// (best-case extraction arm); "olla" (default) uses the local OAI endpoint.
	var completer atom.ClaudeCompleter
	switch cfg.Extractor {
	case "sonnet":
		completer = &sonnetCompleterAdapter{retries: cfg.Retries}
		log.Printf("atom-build: extractor=sonnet (claude --print)")
	case "", "olla":
		completer = &oaiCompleterAdapter{baseURL: cfg.LLMBaseURL, model: cfg.LLMModel, retries: cfg.Retries}
		log.Printf("atom-build: extractor=olla (%s)", cfg.LLMModel)
	default:
		_, _ = fmt.Fprintf(stderr, "invalid --extractor %q: must be olla or sonnet\n", cfg.Extractor)
		return 1
	}
	extractor := atom.NewClaudeExtractor(completer)

	// Build the atom storage function.
	// --direct-db: write directly to Postgres (for use when the REST /atoms endpoint
	// is not yet deployed on the target Engram server).
	// Default: POST /atoms via the Engram REST API.
	var storeAtom atomStoreFunc
	if cfg.DatabaseURL != "" {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
		sharedPool, poolErr := db.NewSharedPool(dbCtx, cfg.DatabaseURL)
		dbCancel()
		if poolErr != nil {
			_, _ = fmt.Fprintf(stderr, "atom-build --direct-db: connect failed: %v\n", poolErr)
			return 1
		}
		defer sharedPool.Close()
		// Use a single PostgresBackend with the _shared project slug for DDL.
		// InsertAtom / InsertAtomEmbedding are project-agnostic (project is a column).
		pg, pgErr := db.NewPostgresBackendWithPool(context.Background(), "_atom_build", sharedPool)
		if pgErr != nil {
			_, _ = fmt.Fprintf(stderr, "atom-build --direct-db: backend failed: %v\n", pgErr)
			return 1
		}
		storeAtom = makeDirectDBStoreFunc(pg)
		log.Printf("atom-build: using direct-db storage path")
	} else {
		serverURL := cfg.ServerURL
		if serverURL == "" {
			serverURL = longmemeval.DefaultServerURL()
		}
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = longmemeval.DefaultAPIKey()
		}
		storeAtom = makeRESTStoreFunc(serverURL, apiKey)
		log.Printf("atom-build: using REST storage path at %s", serverURL)
	}

	// Checkpoint writer.
	ckptCh := make(chan atomBuildEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	var writerErr error
	go func() {
		defer wgWriter.Done()
		if err := writeAtomBuildCheckpoint(buildPath, ckptCh); err != nil {
			writerErr = err
		}
	}()

	// Work queue: only entries not already done.
	var work []atomBuildWorkItem
	for _, e := range ingested {
		if e.Status != "done" || skip[e.QuestionID] {
			continue
		}
		it, ok := itemByID[e.QuestionID]
		if !ok {
			log.Printf("WARN atom-build: question_id %s not found in dataset — skipping", e.QuestionID)
			continue
		}
		work = append(work, atomBuildWorkItem{entry: e, item: it})
	}
	log.Printf("atom-build: %d sessions to process", len(work))

	workCh := make(chan atomBuildWorkItem, len(work)+1)
	for _, w := range work {
		workCh <- w
	}
	close(workCh)

	// Ensure atom cache directory exists if specified.
	if cfg.AtomCacheDir != "" {
		if mkErr := os.MkdirAll(cfg.AtomCacheDir, 0o755); mkErr != nil {
			_, _ = fmt.Fprintf(stderr, "atom-build: create atom-cache-dir: %v\n", mkErr)
			return 1
		}
	}

	stats := &atomBuildStats{}
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomBuildWorker(extractor, embedClient, storeAtom, cfg.AtomCacheDir, cfg.MaxSessions, workCh, ckptCh, stats)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	if writerErr != nil {
		_, _ = fmt.Fprintf(stderr, "atom-build checkpoint write error: %v\n", writerErr)
		return 1
	}
	if exit := atomBuildExitCode(stats, stderr); exit != 0 {
		return exit
	}
	log.Printf("atom-build: complete")
	return 0
}

type atomBuildStats struct {
	processed atomic.Int64
	stored    atomic.Int64
}

func atomBuildExitCode(stats *atomBuildStats, stderr io.Writer) int {
	processed := stats.processed.Load()
	if processed == 0 || stats.stored.Load() > 0 {
		return 0
	}

	_, _ = fmt.Fprintf(
		stderr,
		"atom-build: processed %d sessions but stored 0 atoms; check extraction, --embed-url, and atom storage errors above\n",
		processed,
	)
	return 1
}

// atomBuildWorkItem is the unit of work for atomBuildWorker.
type atomBuildWorkItem struct {
	entry longmemeval.IngestEntry
	item  longmemeval.Item
}

// atomStoreFunc stores one atom + embedding. Implementations: REST and direct-DB.
type atomStoreFunc func(project string, a *atom.Atom, vec []float32) error

// atomBuildWorker processes sessions from workCh, extracts atoms via olla,
// embeds them, and stores them in Engram.
func atomBuildWorker(
	extractor atom.Extractor,
	embedClient embed.Client,
	storeAtom atomStoreFunc,
	atomCacheDir string,
	maxSessions int,
	workCh <-chan atomBuildWorkItem,
	ckptCh chan<- atomBuildEntry,
	stats *atomBuildStats,
) {
	for w := range workCh {
		stats.processed.Add(1)
		entry := w.entry
		item := w.item

		result := atomBuildEntry{
			QuestionID: entry.QuestionID,
			Project:    entry.Project,
		}

		// Extract PER-SESSION, not from a truncated concatenation. The haystack
		// for one LME question is ~470 sessions / up to 5 MB of text; the previous
		// concat-then-truncate(6000) approach only ever saw the first ~2 sessions,
		// so the answer session (median offset ~2.7 MB) was never read and no
		// gold-relevant atom could be extracted. Per-session extraction mirrors the
		// production atom worker (internal/atom/worker.go extracts per memory).
		sessionTexts := buildSessionTexts(item, maxSessions)
		if len(sessionTexts) == 0 {
			result.Status = "error"
			result.Error = "no session text"
			ckptCh <- result
			continue
		}

		var atoms []atom.Atom
		var extractErr error
		for _, st := range sessionTexts {
			if st.text == "" {
				continue
			}
			// 5-minute cap per session. Each session is small (well under the
			// extractor's 6000-char window), so this is generous.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			sa, err := extractor.Extract(ctx, st.text, st.date)
			cancel()
			if err != nil {
				// Record the first error but keep going — one bad session must not
				// zero out the whole question's atoms.
				if extractErr == nil {
					extractErr = err
				}
				log.Printf("WARN atom-build [%s] session %s extract: %v", entry.QuestionID, st.sessionID, err)
				continue
			}
			// Stamp provenance: the session ID this atom came from.
			for i := range sa {
				sa[i].ProvenanceSpan = "session:" + st.sessionID
			}
			atoms = append(atoms, sa...)
		}
		result.AtomsFound = len(atoms)
		if len(atoms) == 0 && extractErr != nil {
			result.Status = "error"
			result.Error = extractErr.Error()
			ckptCh <- result
			continue
		}

		ok := 0
		for i := range atoms {
			a := &atoms[i]
			a.Project = entry.Project
			// Best-effort: link to the first memory in the project as provenance.
			for memID := range entry.MemoryMap {
				a.ProvenanceMemoryID = memID
				break
			}

			embedCtx, embedCancel := context.WithTimeout(context.Background(), 30*time.Second)
			vec, embedErr := embedClient.Embed(embedCtx, a.Statement)
			embedCancel()
			if embedErr != nil {
				log.Printf("WARN atom-build [%s] embed: %v", entry.QuestionID, embedErr)
				continue
			}

			if storeErr := storeAtom(entry.Project, a, vec); storeErr != nil {
				log.Printf("WARN atom-build [%s] store: %v", entry.QuestionID, storeErr)
				continue
			}
			ok++
			stats.stored.Add(1)
		}
		result.AtomsOK = ok
		result.Status = "done"
		ckptCh <- result
		log.Printf("atom-build [%s] project=%s atoms=%d/%d (sessions=%d)", entry.QuestionID, entry.Project, ok, len(atoms), len(sessionTexts))

		// Write atom cache file for the --atom-cache-dir fallback.
		if atomCacheDir != "" && len(atoms) > 0 {
			if cacheErr := writeAtomCacheFile(atomCacheDir, entry.Project, atoms); cacheErr != nil {
				log.Printf("WARN atom-build [%s] cache write: %v", entry.QuestionID, cacheErr)
			}
		}
	}
}

// sessionText pairs a haystack session ID with its formatted text.
type sessionText struct {
	sessionID string
	text      string
	date      time.Time
}

// buildSessionTexts formats each haystack session independently so the extractor
// reads every session (not just the first 6000 chars of a giant concatenation).
//
// maxSessions caps how many sessions are extracted per question (0 = all). When
// capped, the answer session(s) are ALWAYS included (so the best-case validation
// is meaningful), and the remainder are filled deterministically from the front
// of the haystack to provide realistic non-gold preference noise.
func buildSessionTexts(item longmemeval.Item, maxSessions int) []sessionText {
	format := func(session []longmemeval.Turn) string {
		var sb strings.Builder
		for _, turn := range session {
			sb.WriteString(turn.Role + ": " + turn.Content + "\n")
		}
		return sb.String()
	}

	all := make([]sessionText, 0, len(item.HaystackSessions))
	for i, session := range item.HaystackSessions {
		sid := ""
		if i < len(item.HaystackSessionIDs) {
			sid = item.HaystackSessionIDs[i]
		}
		var sessionDate time.Time
		if i < len(item.HaystackDates) {
			sessionDate, _ = parseLongMemEvalQuestionDate(item.HaystackDates[i])
		}
		all = append(all, sessionText{sessionID: sid, text: format(session), date: sessionDate})
	}

	if maxSessions <= 0 || len(all) <= maxSessions {
		return all
	}

	answerIDs := make(map[string]bool, len(item.AnswerSessionIDs))
	for _, id := range item.AnswerSessionIDs {
		answerIDs[id] = true
	}
	picked := make([]sessionText, 0, maxSessions)
	seen := make(map[string]bool)
	// Always include answer sessions first.
	for _, st := range all {
		if answerIDs[st.sessionID] && !seen[st.sessionID] {
			picked = append(picked, st)
			seen[st.sessionID] = true
		}
	}
	// Fill the rest deterministically from the front.
	for _, st := range all {
		if len(picked) >= maxSessions {
			break
		}
		if !seen[st.sessionID] {
			picked = append(picked, st)
			seen[st.sessionID] = true
		}
	}
	return picked
}

// makeRESTStoreFunc returns an atomStoreFunc that stores via POST /atoms.
func makeRESTStoreFunc(serverURL, apiKey string) atomStoreFunc {
	return func(project string, a *atom.Atom, vec []float32) error {
		return storeAtomREST(serverURL, apiKey, project, a, vec)
	}
}

// makeDirectDBStoreFunc returns an atomStoreFunc that writes directly to Postgres
// via a *db.PostgresBackend. Used when the REST /atoms endpoint is not yet deployed.
func makeDirectDBStoreFunc(pg *db.PostgresBackend) atomStoreFunc {
	return func(project string, a *atom.Atom, vec []float32) error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		a.Project = project
		if err := pg.InsertAtom(ctx, a); err != nil {
			return fmt.Errorf("InsertAtom: %w", err)
		}
		if len(vec) > 0 {
			if err := pg.InsertAtomEmbedding(ctx, a.ID, vec); err != nil {
				log.Printf("WARN InsertAtomEmbedding (non-fatal): %v", err)
			}
		}
		return nil
	}
}

// atomStoreRequest is the JSON body sent to POST /atoms.
type atomStoreRequest struct {
	Action    string     `json:"action"`
	Project   string     `json:"project"`
	Atom      *atom.Atom `json:"atom"`
	Embedding []float32  `json:"embedding,omitempty"`
}

// atomHTTPClient is shared across storeAtomREST calls.
var atomHTTPClient = &http.Client{Timeout: 30 * time.Second}

// storeAtomREST calls POST /atoms to store the atom + embedding via the Engram REST API.
func storeAtomREST(serverURL, apiKey, project string, a *atom.Atom, vec []float32) error {
	body, err := json.Marshal(atomStoreRequest{
		Action:    "store",
		Project:   project,
		Atom:      a,
		Embedding: vec,
	})
	if err != nil {
		return fmt.Errorf("marshal atom store request: %w", err)
	}

	url := strings.TrimRight(serverURL, "/") + "/atoms"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build atom store request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := atomHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("atom store POST: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("atom store: unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// writeAtomCacheFile writes atoms as JSON to <dir>/<project>.json.
// Used by --atom-cache-dir to enable local fallback for run --atom-mode.
// The file is overwritten on each call (idempotent).
func writeAtomCacheFile(dir, project string, atoms []atom.Atom) error {
	path := filepath.Join(dir, project+".json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(atoms)
}

// readAtomBuildSkipSet reads checkpoint-atom-build.jsonl and returns the set
// of question IDs with status == "done".
func readAtomBuildSkipSet(path string) (map[string]bool, error) {
	skip := make(map[string]bool)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return skip, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	dec := json.NewDecoder(f)
	for {
		var e atomBuildEntry
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if e.Status == "done" {
			skip[e.QuestionID] = true
		}
	}
	return skip, nil
}

// writeAtomBuildCheckpoint writes entries from ch to path as JSONL.
func writeAtomBuildCheckpoint(path string, ch <-chan atomBuildEntry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		for range ch {
		}
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for e := range ch {
		_ = enc.Encode(e)
	}
	return nil
}
