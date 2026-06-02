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
	combined := system + "\n\n" + prompt
	return longmemeval.GenerateOAI(ctx, combined, a.baseURL, a.model, a.retries)
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
	if cfg.LLMBaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "--llm-url is required (local olla endpoint)")
		return 1
	}
	if cfg.LLMModel == "" {
		_, _ = fmt.Fprintln(stderr, "--llm-model is required")
		return 1
	}
	if cfg.EmbedURL == "" {
		cfg.EmbedURL = cfg.LLMBaseURL
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = "BAAI/bge-m3"
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

	// Build extractor backed by olla.
	completer := &oaiCompleterAdapter{baseURL: cfg.LLMBaseURL, model: cfg.LLMModel, retries: cfg.Retries}
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
			serverURL = defaultServerURL()
		}
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = defaultAPIKey()
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

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomBuildWorker(extractor, embedClient, storeAtom, cfg.AtomCacheDir, workCh, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	if writerErr != nil {
		_, _ = fmt.Fprintf(stderr, "atom-build checkpoint write error: %v\n", writerErr)
		return 1
	}
	log.Printf("atom-build: complete")
	return 0
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
	embedClient *embed.LiteLLMClient,
	storeAtom atomStoreFunc,
	atomCacheDir string,
	workCh <-chan atomBuildWorkItem,
	ckptCh chan<- atomBuildEntry,
) {
	for w := range workCh {
		entry := w.entry
		item := w.item

		result := atomBuildEntry{
			QuestionID: entry.QuestionID,
			Project:    entry.Project,
		}

		sessionText := buildSessionText(item)
		if sessionText == "" {
			result.Status = "error"
			result.Error = "no session text"
			ckptCh <- result
			continue
		}

		// 10-minute cap: slow reasoning models (Qwen3 thinking via olla) can take
		// several minutes on long session concatenations. Must exceed the embed
		// client's internal timeout headroom; the GenerateOAI call inside Extract
		// has its own 600s per-attempt cap (generateTimeout).
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		atoms, err := extractor.Extract(ctx, sessionText)
		cancel()
		if err != nil {
			log.Printf("WARN atom-build [%s] extract: %v", entry.QuestionID, err)
			result.Status = "error"
			result.Error = err.Error()
			ckptCh <- result
			continue
		}
		result.AtomsFound = len(atoms)

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
		}
		result.AtomsOK = ok
		result.Status = "done"
		ckptCh <- result
		log.Printf("atom-build [%s] project=%s atoms=%d/%d", entry.QuestionID, entry.Project, ok, len(atoms))

		// Write atom cache file for the --atom-cache-dir fallback.
		if atomCacheDir != "" && len(atoms) > 0 {
			if cacheErr := writeAtomCacheFile(atomCacheDir, entry.Project, atoms); cacheErr != nil {
				log.Printf("WARN atom-build [%s] cache write: %v", entry.QuestionID, cacheErr)
			}
		}
	}
}

// buildSessionText concatenates all haystack session turns into a single text blob.
func buildSessionText(item longmemeval.Item) string {
	var sb strings.Builder
	for i, session := range item.HaystackSessions {
		if i < len(item.HaystackSessionIDs) {
			sb.WriteString("Session: " + item.HaystackSessionIDs[i] + "\n")
		}
		for _, turn := range session {
			sb.WriteString(turn.Role + ": " + turn.Content + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
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
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
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
