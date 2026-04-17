package mcp

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

// Tier classifies an incoming document by size so the caller can pick a
// storage strategy.
type Tier int

const (
	// TierSmall — content fits in the standard MaxContentLength budget; no
	// synopsis required. Use the existing focused/document storage path.
	TierSmall Tier = iota
	// TierStreamSynopsis — content exceeds MaxContentLength but fits within
	// MaxDocumentBytes; store a synopsis as Memory.Content and chunk the full
	// raw body inline.
	TierStreamSynopsis
	// TierRawDocument — content exceeds MaxDocumentBytes but fits within
	// RawDocumentMaxBytes; park the full body in the documents table and
	// keep the memory as a synopsis referencing the document_id.
	TierRawDocument
	// TierReject — content is larger than RawDocumentMaxBytes and must be
	// refused.
	TierReject
)

// classifyDocumentSize picks a storage tier from a byte count and configured
// caps. Kept pure so routing can be exercised without a database.
func classifyDocumentSize(size, maxDoc, rawMax int) Tier {
	switch {
	case size <= types.MaxContentLength:
		return TierSmall
	case size <= maxDoc:
		return TierStreamSynopsis
	case size <= rawMax:
		return TierRawDocument
	default:
		return TierReject
	}
}

// synopsisPrefixBytes is the number of leading bytes preserved verbatim at
// the head of a synopsis.
const synopsisPrefixBytes = 8192

// synopsisHeadingBytes is the maximum number of bytes the extracted heading
// outline may consume in a synopsis.
const synopsisHeadingBytes = 2048

// buildSynopsis returns a condensed representation of content suitable for
// storage in Memory.Content when the full body is too large. Formula:
//   - leading 8 KiB verbatim
//   - plus the heading outline (lines matching ^#+ \S), capped at 2 KiB
//
// The two sections are joined by "\n\n--- Outline ---\n" so a reader can tell
// where verbatim text ends and the outline begins. When content fits in 8 KiB
// the function returns content unchanged.
func buildSynopsis(content string) string {
	if len(content) <= synopsisPrefixBytes {
		return content
	}
	prefix := content[:synopsisPrefixBytes]

	// Extract heading lines. We scan line-by-line from the full content so
	// headings beyond the 8 KiB prefix still surface in the outline.
	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(content))
	// Some markdown documents have very long lines; raise the scanner buffer
	// so we don't drop headings that sit on such lines.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	remaining := synopsisHeadingBytes
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// A valid ATX heading must have one or more '#' then a space.
		hashEnd := 0
		for hashEnd < len(trimmed) && trimmed[hashEnd] == '#' {
			hashEnd++
		}
		if hashEnd == 0 || hashEnd >= len(trimmed) || trimmed[hashEnd] != ' ' {
			continue
		}
		entry := trimmed + "\n"
		if len(entry) > remaining {
			break
		}
		out.WriteString(entry)
		remaining -= len(entry)
	}
	outline := strings.TrimRight(out.String(), "\n")
	if outline == "" {
		return prefix
	}
	return prefix + "\n\n--- Outline ---\n" + outline
}

// documentStorer is the narrow backend surface execStoreDocument needs. Kept
// as a separate interface so tests can inject an in-memory stub instead of a
// full PostgresBackend.
type documentStorer interface {
	StoreDocument(ctx context.Context, project, content string) (string, error)
	SetMemoryDocumentID(ctx context.Context, memoryID, documentID string) error
}

// memoryStorer is the engine-side surface needed to persist a memory. Both
// *search.SearchEngine and test stubs can satisfy it.
type memoryStorer interface {
	StoreWithRawBody(ctx context.Context, m *types.Memory, rawBody string) error
}

// storeDocumentDeps bundles the collaborators required by execStoreDocument.
// Keeping them in a struct makes the test seams visible and keeps the handler
// itself short.
type storeDocumentDeps struct {
	engine  memoryStorer
	backend documentStorer
}

// execStoreDocument is the testable core of handleMemoryStoreDocument. It
// classifies the content by size, builds a synopsis when required, stores the
// memory via the engine (which handles chunking + embedding), and — for the
// Tier-2 path — stores the raw body in the documents table first so the FK
// is available.
//
// Returns the serialisable result map that the MCP handler marshals to JSON.
func execStoreDocument(ctx context.Context, deps storeDocumentDeps, m *types.Memory, content string, maxDoc, rawMax int) (map[string]any, error) {
	tier := classifyDocumentSize(len(content), maxDoc, rawMax)
	switch tier {
	case TierReject:
		return nil, fmt.Errorf("document exceeds maximum size (%d bytes > %d)", len(content), rawMax)

	case TierSmall:
		m.Content = content
		m.StorageMode = "document"
		if err := deps.engine.StoreWithRawBody(ctx, m, ""); err != nil {
			return nil, err
		}
		return map[string]any{
			"id":         m.ID,
			"status":     "stored",
			"mode":       "document",
			"size_bytes": len(content),
		}, nil

	case TierStreamSynopsis:
		synopsis := buildSynopsis(content)
		m.Content = synopsis
		m.StorageMode = "document"
		// Chunk the full body so recall stays grounded.
		if err := deps.engine.StoreWithRawBody(ctx, m, content); err != nil {
			return nil, err
		}
		return map[string]any{
			"id":              m.ID,
			"status":          "stored",
			"mode":            "document_synopsis",
			"size_bytes":      len(content),
			"synopsis_bytes":  len(synopsis),
		}, nil

	case TierRawDocument:
		// Step 1: park the raw body so the FK is available before we write
		// the memory row.
		docID, err := deps.backend.StoreDocument(ctx, m.Project, content)
		if err != nil {
			return nil, fmt.Errorf("store raw document: %w", err)
		}
		// Step 2: build synopsis + store memory (no raw-body chunking — raw
		// documents are recalled by synopsis embedding and queried by the
		// A5 memory_query_document tool).
		synopsis := buildSynopsis(content)
		m.Content = synopsis
		m.StorageMode = "document"
		m.DocumentID = docID
		if err := deps.engine.StoreWithRawBody(ctx, m, ""); err != nil {
			return nil, fmt.Errorf("store synopsis memory: %w", err)
		}
		// Step 3: belt-and-braces FK link. Engine.Store inserts the memory
		// before we have document_id threaded through the INSERT statement,
		// so we set it after the fact. Same reasoning as the raw-body split.
		if err := deps.backend.SetMemoryDocumentID(ctx, m.ID, docID); err != nil {
			return nil, fmt.Errorf("link memory to document: %w", err)
		}
		return map[string]any{
			"id":          m.ID,
			"document_id": docID,
			"status":      "stored",
			"mode":        "raw_document",
			"size_bytes":  len(content),
		}, nil
	}
	return nil, fmt.Errorf("unreachable tier %d", tier)
}

// configOrDefaults returns MaxDocumentBytes/RawDocumentMaxBytes with built-in
// fallbacks so handlers never operate on zero caps (which would misclassify
// every document as TierReject).
func configOrDefaults(cfg Config) (maxDoc, rawMax int) {
	maxDoc = cfg.MaxDocumentBytes
	if maxDoc <= 0 {
		maxDoc = 8 * 1024 * 1024
	}
	rawMax = cfg.RawDocumentMaxBytes
	if rawMax <= 0 {
		rawMax = 50 * 1024 * 1024
	}
	return
}

// engineStorerAdapter lets a *search.SearchEngine satisfy memoryStorer without
// declaring the interface in the search package (avoids an import cycle).
type engineStorerAdapter struct {
	store func(ctx context.Context, m *types.Memory, rawBody string) error
}

func (a engineStorerAdapter) StoreWithRawBody(ctx context.Context, m *types.Memory, rawBody string) error {
	return a.store(ctx, m, rawBody)
}

// backendDocumentAdapter narrows a db.Backend to the documentStorer surface.
type backendDocumentAdapter struct {
	b db.Backend
}

func (a backendDocumentAdapter) StoreDocument(ctx context.Context, project, content string) (string, error) {
	return a.b.StoreDocument(ctx, project, content)
}
func (a backendDocumentAdapter) SetMemoryDocumentID(ctx context.Context, memoryID, documentID string) error {
	return a.b.SetMemoryDocumentID(ctx, memoryID, documentID)
}

// ── memory_ingest_document_stream tool ──────────────────────────────────────

// uploadSession holds assembled parts for an in-flight chunked upload. Keyed
// by caller-chosen upload_id in uploadRegistry.
type uploadSession struct {
	mu      sync.Mutex
	project string
	buf     []byte
	// nextPart is the expected index for the next Write; enforces ordering so
	// callers cannot silently produce a corrupt blob by re-ordering parts.
	nextPart int
}

// uploadRegistry is process-local — chunked uploads do not survive a restart.
// This matches the intent (large documents should be ingested in one burst or
// retried from scratch on failure).
var uploadRegistry = struct {
	mu       sync.Mutex
	sessions map[string]*uploadSession
}{sessions: make(map[string]*uploadSession)}

func getOrCreateUpload(uploadID, project string) *uploadSession {
	uploadRegistry.mu.Lock()
	defer uploadRegistry.mu.Unlock()
	if s, ok := uploadRegistry.sessions[uploadID]; ok {
		return s
	}
	s := &uploadSession{project: project}
	uploadRegistry.sessions[uploadID] = s
	return s
}

func dropUpload(uploadID string) {
	uploadRegistry.mu.Lock()
	defer uploadRegistry.mu.Unlock()
	delete(uploadRegistry.sessions, uploadID)
}

// handleMemoryIngestDocumentStream exposes two ingestion modes:
//
//  1. Server-local file — pass {path: "..."} and the server reads the file
//     directly. Subject to the same DataDir sandbox as other file tools.
//  2. Chunked upload — pass {upload_id, part, data (base64), final} across
//     multiple calls. The final call (final=true) assembles and ingests.
//
// Either mode ends in a call to execStoreDocument so Tier-1 / Tier-2 routing
// is shared with handleMemoryStoreDocument.
func handleMemoryIngestDocumentStream(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	maxDoc, rawMax := configOrDefaults(cfg)

	path := getString(args, "path", "")
	if path != "" {
		// Server-local file mode.
		if cfg.DataDir == "" {
			return nil, fmt.Errorf("path mode requires --data-dir / ENGRAM_DATA_DIR to be set")
		}
		safe, err := SafePath(cfg.DataDir, path)
		if err != nil {
			return nil, fmt.Errorf("invalid path: %w", err)
		}
		info, err := os.Stat(safe)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", path, err)
		}
		if info.Size() > int64(rawMax) {
			return nil, fmt.Errorf("document exceeds maximum size (%d bytes > %d)", info.Size(), rawMax)
		}
		data, err := os.ReadFile(safe)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", path, err)
		}
		return runStreamIngest(ctx, pool, project, string(data), cfg, maxDoc, rawMax)
	}

	// Chunked-upload mode.
	uploadID := getString(args, "upload_id", "")
	if uploadID == "" {
		return nil, fmt.Errorf("either path or upload_id is required")
	}
	part := getInt(args, "part", -1)
	if part < 0 {
		return nil, fmt.Errorf("part is required for chunked uploads (0-indexed)")
	}
	b64Data, _ := args["data"].(string)
	final := getBool(args, "final", false)

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, fmt.Errorf("data must be base64-encoded: %w", err)
	}

	sess := getOrCreateUpload(uploadID, project)
	sess.mu.Lock()
	if part != sess.nextPart {
		sess.mu.Unlock()
		return nil, fmt.Errorf("part out of order: expected %d, got %d", sess.nextPart, part)
	}
	if len(sess.buf)+len(decoded) > rawMax {
		sess.mu.Unlock()
		dropUpload(uploadID)
		return nil, fmt.Errorf("document exceeds maximum size (%d bytes > %d)", len(sess.buf)+len(decoded), rawMax)
	}
	sess.buf = append(sess.buf, decoded...)
	sess.nextPart++
	if !final {
		received := len(sess.buf)
		sess.mu.Unlock()
		return toolResult(map[string]any{
			"upload_id":      uploadID,
			"status":         "buffered",
			"parts_received": part + 1,
			"bytes_received": received,
		})
	}
	body := string(sess.buf)
	sess.mu.Unlock()
	dropUpload(uploadID)
	return runStreamIngest(ctx, pool, project, body, cfg, maxDoc, rawMax)
}

// runStreamIngest funnels a fully assembled body into execStoreDocument and
// wraps the result in an MCP tool response.
func runStreamIngest(ctx context.Context, pool *EnginePool, project, body string, cfg Config, maxDoc, rawMax int) (*mcpgo.CallToolResult, error) {
	_ = cfg // reserved for future options (e.g. memory_type override)
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		MemoryType: types.MemoryTypeContext,
		Project:    project,
		Importance: 2,
	}
	engine := h.Engine
	deps := storeDocumentDeps{
		engine: engineStorerAdapter{store: engine.StoreWithRawBody},
		backend: backendDocumentAdapter{b: engine.Backend()},
	}
	out, err := execStoreDocument(ctx, deps, m, body, maxDoc, rawMax)
	if err != nil {
		return nil, err
	}
	// Expose memory_id / document_id under the names the spec calls for.
	renamed := map[string]any{
		"memory_id":  out["id"],
		"size_bytes": out["size_bytes"],
	}
	if docID, ok := out["document_id"]; ok {
		renamed["document_id"] = docID
	}
	if syn, ok := out["synopsis_bytes"]; ok {
		renamed["summary_bytes"] = syn
	}
	renamed["status"] = out["status"]
	renamed["mode"] = out["mode"]
	return toolResult(renamed)
}
