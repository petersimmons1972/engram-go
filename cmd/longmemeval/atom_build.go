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
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// AtomBuildConfig holds flags for the atom-build subcommand.
type AtomBuildConfig struct {
	DataFile   string
	OutDir     string
	RunID      string
	Workers    int
	ServerURL  string
	APIKey     string
	LLMBaseURL string
	LLMModel   string
	EmbedURL   string
	EmbedModel string
	Retries    int
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

	// Build the REST client for atom + embedding storage.
	serverURL := cfg.ServerURL
	if serverURL == "" {
		serverURL = defaultServerURL()
	}
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = defaultAPIKey()
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

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomBuildWorker(extractor, embedClient, serverURL, apiKey, workCh, ckptCh)
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

// atomBuildWorker processes sessions from workCh, extracts atoms via olla,
// embeds them, and stores them in Engram.
func atomBuildWorker(
	extractor atom.Extractor,
	embedClient *embed.LiteLLMClient,
	serverURL, apiKey string,
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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

			if storeErr := storeAtomREST(serverURL, apiKey, entry.Project, a, vec); storeErr != nil {
				log.Printf("WARN atom-build [%s] store: %v", entry.QuestionID, storeErr)
				continue
			}
			ok++
		}
		result.AtomsOK = ok
		result.Status = "done"
		ckptCh <- result
		log.Printf("atom-build [%s] project=%s atoms=%d/%d", entry.QuestionID, entry.Project, ok, len(atoms))
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
