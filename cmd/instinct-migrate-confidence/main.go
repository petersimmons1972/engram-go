// instinct-migrate-confidence detects and optionally corrects Engram memory
// records where the Python instinct consolidator stored importance as an
// integer 1–10 instead of a float 0.0–1.0.
//
// Modes:
//
//	--detect-and-report  (default) Read-only. Prints a JSON report to stdout
//	                     and a human summary to stderr.
//	--backup-only        Dump all instinct-tagged records to a JSONL file.
//	                     Required before --apply.
//	--apply              Apply corrections (requires backup to exist first).
//	--revert <logfile>   Reverse a previous --apply run using its migration log.
//
// The three modes --detect-and-report, --backup-only, and --apply are mutually
// exclusive. --revert may be combined only with path flags.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// allProjects is the canonical list of Engram projects to scan.
// Matches CLAUDE.md project list plus "psimmons" from the audit binary.
var allProjects = []string{
	"psimmons",
	"global",
	"clearwatch",
	"homelab",
	"engram",
	"3dprint",
	"family",
}

// engramRecord is the minimal shape of an Engram memory record.
type engramRecord struct {
	ID         string   `json:"id"`
	Project    string   `json:"project"`
	Importance float64  `json:"importance"`
	Tags       []string `json:"tags"`
	Content    string   `json:"content,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

// engramClient is the interface that detect, apply, backup, and revert use.
// The real implementation calls Engram REST + MCP; tests use a mock.
type engramClient interface {
	// queryRecords returns all instinct-tagged records in a project.
	// Implementations may use /quick-recall with broad query terms.
	queryRecords(ctx context.Context, project string) ([]engramRecord, error)
	// correctRecord updates the importance field of a memory record.
	correctRecord(ctx context.Context, id, project string, importance float64) error
	// fetchRecord retrieves the current state of a single record by ID.
	// Used for idempotency checks immediately before correction.
	fetchRecord(ctx context.Context, id, project string) (*engramRecord, error)
}

// nowUTC returns the current time as an RFC3339 string.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ── Real Engram client ───────────────────────────────────────────────────────

// httpEngram is the production engramClient. It uses:
//   - /quick-recall for reads (sessionless REST endpoint)
//   - /quick-recall with id filter for fetchRecord
//   - MCP streamable HTTP POST /mcp for memory_correct writes
type httpEngram struct {
	baseURL string
	token   string
	hc      *http.Client
}

func newHTTPEngram(baseURL, token string) *httpEngram {
	return &httpEngram{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		hc:      &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *httpEngram) doPost(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// queryRecords queries /quick-recall for instinct pattern records in a project.
// Uses the same query string as the Python audit binary's fetchPatterns.
func (e *httpEngram) queryRecords(ctx context.Context, project string) ([]engramRecord, error) {
	body := map[string]any{
		"query":   "PROVENANCE observed instinct behaviour pattern first seen",
		"project": project,
		"limit":   200,
	}
	data, err := e.doPost(ctx, "/quick-recall", body)
	if err != nil {
		return nil, fmt.Errorf("quick-recall %s: %w", project, err)
	}
	var payload struct {
		Results []struct {
			ID         string   `json:"id"`
			Importance float64  `json:"importance"`
			Tags       []string `json:"tags"`
			Content    string   `json:"content"`
			Summary    string   `json:"summary"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode quick-recall %s: %w", project, err)
	}
	out := make([]engramRecord, 0, len(payload.Results))
	for _, r := range payload.Results {
		out = append(out, engramRecord{
			ID:         r.ID,
			Project:    project,
			Importance: r.Importance,
			Tags:       r.Tags,
			Content:    r.Content,
			Summary:    r.Summary,
		})
	}
	return out, nil
}

// fetchRecord retrieves a single record's current importance by re-querying
// the project and filtering by ID. This avoids needing a dedicated /fetch
// REST endpoint (which requires an MCP session).
func (e *httpEngram) fetchRecord(ctx context.Context, id, project string) (*engramRecord, error) {
	records, err := e.queryRecords(ctx, project)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("record %s not found in project %s", id, project)
}

// correctRecord calls the MCP memory_correct tool via streamable HTTP POST /mcp.
// The importance is sent as a float64; the server's handleMemoryCorrect accepts
// it via args["importance"].(float64).
func (e *httpEngram) correctRecord(ctx context.Context, id, project string, importance float64) error {
	// MCP streamable HTTP: POST /mcp with JSON-RPC 2.0 body.
	rpcBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "memory_correct",
			"arguments": map[string]any{
				"memory_id":  id,
				"project":    project,
				"importance": importance,
			},
		},
	}
	data, err := e.doPost(ctx, "/mcp", rpcBody)
	if err != nil {
		return fmt.Errorf("memory_correct %s: %w", id, err)
	}
	var rpcResp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &rpcResp); err == nil && rpcResp.Error != nil {
		return fmt.Errorf("memory_correct %s: RPC error: %s", id, rpcResp.Error.Message)
	}
	return nil
}

// ── Config / entrypoint ──────────────────────────────────────────────────────

// resolveEngram reads endpoint and token from flags or ~/.claude/mcp_servers.json.
// Ported verbatim from ~/projects/instinct/cmd/audit/main.go:262-284.
func resolveEngram(baseFlag, tokenFlag string) (string, string) {
	if baseFlag != "" && tokenFlag != "" {
		return baseFlag, tokenFlag
	}
	cfgPath := filepath.Join(os.Getenv("HOME"), ".claude", "mcp_servers.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("read %s: %v", cfgPath, err)
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("parse mcp_servers.json: %v", err)
	}
	e := cfg.McpServers["engram"]
	sseURL := strings.TrimSuffix(strings.TrimRight(e.URL, "/"), "/sse")
	auth := strings.TrimPrefix(e.Headers["Authorization"], "Bearer ")
	return sseURL, auth
}

func main() {
	var (
		flagDetect     = flag.Bool("detect-and-report", false, "Read-only scan and report (default mode)")
		flagApply      = flag.Bool("apply", false, "Apply confidence corrections (requires --backup-only first)")
		flagBackupOnly = flag.Bool("backup-only", false, "Dump all instinct records to backup JSONL file")
		flagRevert     = flag.String("revert", "", "Revert a previous --apply run using its migration log path")
		flagEngram     = flag.String("engram", "", "Engram base URL override (e.g. http://localhost:8788)")
		flagToken      = flag.String("token", "", "Engram API token override")
	)
	flag.Parse()

	// Validate mutual exclusivity.
	modeCount := 0
	if *flagDetect {
		modeCount++
	}
	if *flagApply {
		modeCount++
	}
	if *flagBackupOnly {
		modeCount++
	}
	if *flagRevert != "" {
		modeCount++
	}
	if modeCount > 1 {
		fmt.Fprintln(os.Stderr, "Error: --detect-and-report, --apply, --backup-only, and --revert are mutually exclusive")
		os.Exit(1)
	}

	// Default mode is --detect-and-report.
	if modeCount == 0 {
		*flagDetect = true
	}

	baseURL, token := resolveEngram(*flagEngram, *flagToken)
	client := newHTTPEngram(baseURL, token)

	home := os.Getenv("HOME")
	stateDir := filepath.Join(home, ".local", "state", "instinct")
	backupDir := filepath.Join(stateDir, "backups")
	date := time.Now().Format("2006-01-02")

	ctx := context.Background()

	switch {
	case *flagDetect:
		rpt, err := runDetect(ctx, client, allProjects)
		if err != nil {
			log.Fatalf("detect: %v", err)
		}
		if err := defaultWriteReport(rpt); err != nil {
			log.Fatalf("write report: %v", err)
		}

	case *flagBackupOnly:
		if err := runBackup(ctx, client, allProjects, backupDir, date); err != nil {
			log.Fatalf("backup: %v", err)
		}

	case *flagApply:
		cfg := applyConfig{
			backupDir: backupDir,
			logDir:    stateDir,
			date:      date,
		}
		if err := runApply(ctx, client, allProjects, cfg); err != nil {
			log.Fatalf("apply: %v", err)
		}

	case *flagRevert != "":
		cfg := revertConfig{
			logDir: stateDir,
			date:   date,
		}
		if err := runRevert(ctx, client, *flagRevert, cfg); err != nil {
			log.Fatalf("revert: %v", err)
		}
	}
}
