package mcp

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	// Walk backward from the naive byte cut to the nearest UTF-8 rune start so
	// we never emit a synopsis that splits a multi-byte rune. Postgres rejects
	// invalid UTF-8 on INSERT, and a mid-rune cut is the easiest way to hit
	// that failure in the wild (any document containing non-ASCII at the
	// boundary triggers it).
	cut := synopsisPrefixBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	prefix := content[:cut]

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
			"id":         m.ID,
			"status":     "stored",
			"mode":       "document_synopsis",
			"size_bytes": len(content),
			"summary":    synopsis,
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
		// A5 memory_query_document tool). StoreMemoryTx now writes
		// document_id in the INSERT so the link is set atomically with the
		// memory row itself.
		synopsis := buildSynopsis(content)
		m.Content = synopsis
		m.StorageMode = "document"
		m.DocumentID = docID
		if err := deps.engine.StoreWithRawBody(ctx, m, ""); err != nil {
			return nil, fmt.Errorf("store synopsis memory: %w", err)
		}
		// Step 3: belt-and-braces FK link. The INSERT in step 2 already sets
		// document_id, so this UPDATE is now redundant in the happy path.
		// Kept as best-effort cleanup in case a future path creates the
		// memory without the column populated. Logged but not returned —
		// failing here after a successful memory store would leave the
		// caller thinking the ingest failed when the row is actually fine.
		// TODO: remove once all memory-write paths populate document_id.
		if err := deps.backend.SetMemoryDocumentID(ctx, m.ID, docID); err != nil {
			// Best-effort — memory + document are both persisted and the
			// INSERT already linked them. This UPDATE would only matter if
			// a caller bypassed StoreMemoryTx.
			slog.Warn("SetMemoryDocumentID belt-and-braces update failed",
				"memory_id", m.ID, "document_id", docID, "err", err)
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
// by caller-chosen upload_id in Server.uploads.
type uploadSession struct {
	mu      sync.Mutex
	project string
	buf     []byte
	// nextPart is the expected index for the next Write; enforces ordering so
	// callers cannot silently produce a corrupt blob by re-ordering parts.
	nextPart     int
	createdAt    time.Time
	lastActivity time.Time // updated on every successful append; used for TTL eviction (#187)
}

// Caps on the per-Server upload registry. These exist so a misbehaving or
// malicious caller cannot pin unbounded memory by starting sessions and never
// finishing them.
const (
	maxUploadSessions = 500
	uploadSessionTTL  = 30 * time.Minute
	maxUploadIDLen    = 128
)

// uploadIDRE restricts upload_id to safe filesystem-friendly characters.
var uploadIDRE = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// evictExpiredUploadsLocked drops sessions whose lastActivity (falling back to
// createdAt) is older than the TTL. Caller must hold s.uploadMu.
func (s *Server) evictExpiredUploadsLocked(now time.Time) {
	for id, sess := range s.uploads {
		activity := sess.lastActivity
		if activity.IsZero() {
			activity = sess.createdAt
		}
		if now.Sub(activity) > uploadSessionTTL {
			delete(s.uploads, id)
		}
	}
}

// startUploadSession atomically evicts expired sessions, enforces the cap, and
// creates a fresh session under uploadID. Returns an error if the cap has been
// reached or if a session already exists for uploadID.
func (s *Server) startUploadSession(uploadID, project string) (*uploadSession, error) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	s.evictExpiredUploadsLocked(time.Now())
	if _, exists := s.uploads[uploadID]; exists {
		return nil, fmt.Errorf("upload_id already in use")
	}
	if len(s.uploads) >= maxUploadSessions {
		return nil, fmt.Errorf("too many in-progress uploads")
	}
	sess := &uploadSession{project: project, createdAt: time.Now()}
	s.uploads[uploadID] = sess
	return sess, nil
}

// lookupUploadSession returns an existing session or an error if missing.
func (s *Server) lookupUploadSession(uploadID string) (*uploadSession, error) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	s.evictExpiredUploadsLocked(time.Now())
	sess, ok := s.uploads[uploadID]
	if !ok {
		return nil, fmt.Errorf("unknown upload_id %q (expired or never started)", uploadID)
	}
	return sess, nil
}

func (s *Server) dropUpload(uploadID string) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	delete(s.uploads, uploadID)
}

// handleMemoryIngestDocumentStream exposes two ingestion modes:
//
//  1. Server-local file — pass {path: "..."} and the server reads the file
//     directly. Subject to the same DataDir sandbox as other file tools.
//  2. Chunked upload — pass {action: "start"|"append"|"finish", upload_id, ...}
//     across multiple calls. start creates a session, append feeds parts
//     (0-indexed), finish assembles and ingests.
//
// Either mode ends in a call to execStoreDocument so Tier-1 / Tier-2 routing
// is shared with handleMemoryStoreDocument.
//
// s carries the per-Server upload registry. When s is nil (unit tests that only
// exercise the path/file branch) the chunked-upload actions will panic — tests
// must supply a non-nil *Server for those code paths.
func handleMemoryIngestDocumentStream(ctx context.Context, s *Server, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
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

	action := getString(args, "action", "")
	if action == "" {
		return nil, fmt.Errorf("action is required (start|append|finish) when path is not set")
	}
	uploadID := getString(args, "upload_id", "")

	switch action {
	case "start":
		if len(uploadID) == 0 || len(uploadID) > maxUploadIDLen {
			return nil, fmt.Errorf("upload_id must be 1–%d characters", maxUploadIDLen)
		}
		if !uploadIDRE.MatchString(uploadID) {
			return nil, fmt.Errorf("upload_id may only contain letters, digits, hyphens, underscores, and dots")
		}
		if _, err := s.startUploadSession(uploadID, project); err != nil {
			return nil, err
		}
		return toolResult(map[string]any{
			"upload_id": uploadID,
			"status":    "started",
		})

	case "append":
		if uploadID == "" {
			return nil, fmt.Errorf("upload_id is required for append")
		}
		part := getInt(args, "part", -1)
		if part < 0 {
			// Legacy callers may use part_index.
			part = getInt(args, "part_index", -1)
		}
		if part < 0 {
			return nil, fmt.Errorf("part is required for append (0-indexed)")
		}
		// Fix #183: require a string 'data' field; reject missing/wrong-type values
		// that would otherwise silently decode to zero bytes and advance the counter.
		b64Data, ok := args["data"].(string)
		if !ok {
			return nil, fmt.Errorf("action=append requires a string 'data' field")
		}
		decoded, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			return nil, fmt.Errorf("data must be base64-encoded: %w", err)
		}
		sess, err := s.lookupUploadSession(uploadID)
		if err != nil {
			return nil, err
		}
		// Project isolation: reject cross-project appends. A caller who started
		// the upload under project A cannot feed parts under project B and
		// silently ingest into the wrong project on finish. Only enforce when
		// the caller explicitly passed a project arg — an omitted project arg
		// falls through to the session's project without complaint.
		if _, passed := args["project"]; passed && sess.project != "" && project != sess.project {
			return nil, fmt.Errorf("project mismatch: upload started with project %q, got %q", sess.project, project)
		}
		sess.mu.Lock()
		if part != sess.nextPart {
			sess.mu.Unlock()
			return nil, fmt.Errorf("part out of order: expected %d, got %d", sess.nextPart, part)
		}
		if len(sess.buf)+len(decoded) > rawMax {
			// Fix #189: zero buf under the lock before unlocking so a concurrent
			// finish cannot race into runStreamIngest with a truncated body.
			// After unlock the session is also removed from the registry.
			wouldBeSize := len(sess.buf) + len(decoded)
			sess.buf = nil
			sess.mu.Unlock()
			s.dropUpload(uploadID)
			return nil, fmt.Errorf("document exceeds maximum size (%d bytes > %d)", wouldBeSize, rawMax)
		}
		sess.buf = append(sess.buf, decoded...)
		sess.nextPart++
		sess.lastActivity = time.Now() // Fix #187: reset TTL clock on every successful append
		received := len(sess.buf)
		partsReceived := sess.nextPart
		sess.mu.Unlock()
		return toolResult(map[string]any{
			"upload_id":      uploadID,
			"status":         "buffered",
			"parts_received": partsReceived,
			"bytes_received": received,
		})

	case "finish":
		if uploadID == "" {
			return nil, fmt.Errorf("upload_id is required for finish")
		}
		sess, err := s.lookupUploadSession(uploadID)
		if err != nil {
			return nil, err
		}
		// Project isolation: the finish call must match the project the session
		// was started under. Without this, a caller can start under "A" and
		// finish under "B", parking the body in the wrong project. Only
		// enforce when the caller explicitly passed a project arg.
		if _, passed := args["project"]; passed && sess.project != "" && project != sess.project {
			return nil, fmt.Errorf("project mismatch: upload started with project %q, got %q", sess.project, project)
		}
		sess.mu.Lock()
		// Fix #189: guard against a nil buf set by a concurrent overflow-eviction.
		if sess.buf == nil {
			sess.mu.Unlock()
			return nil, fmt.Errorf("upload %q was aborted (size overflow in a concurrent append)", uploadID)
		}
		body := string(sess.buf)
		sess.mu.Unlock()
		s.dropUpload(uploadID)
		return runStreamIngest(ctx, pool, sess.project, body, cfg, maxDoc, rawMax)

	default:
		return nil, fmt.Errorf("unknown action %q (expected start|append|finish)", action)
	}
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
	if syn, ok := out["summary"]; ok {
		renamed["summary"] = syn
	}
	renamed["status"] = out["status"]
	renamed["mode"] = out["mode"]
	return toolResult(renamed)
}
