# Open Issues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve all 6 open GitHub issues: two CI fixes (#264, #265), a security advisory doc (#261), two Python consolidator hardening items (#266, #267), and the new `memory_ingest_export` MCP tool (#262).

**Architecture:** Issues are grouped into four independent tracks that can be worked in parallel: CI fixes land first (unblock clean CI for subsequent PRs), Python consolidator hardening and docs can land in any order, and the MCP feature (#262) has a prerequisite of merging `feat/ingest-parsers` first. Each track produces a single PR.

**Tech Stack:** Go 1.25, Python 3.10+, GitHub Actions, pytest, golangci-lint v2.

---

## File Map

| Issue | Files Modified |
|-------|---------------|
| #264 | `.github/workflows/ci.yml` |
| #265 | `.github/workflows/ci.yml` |
| #261 | `internal/mcp/server.go`, `docs/operations.md` |
| #266 | `consolidator/instinct/engram_client.py`, `consolidator/tests/test_engram_client.py` |
| #267 | `consolidator/instinct/run.py`, `consolidator/tests/test_run.py` |
| #262 | `internal/ingest/router/router.go`, `internal/ingest/router/router_test.go`, `internal/mcp/ingest_export.go` *(new)*, `internal/mcp/ingest_export_test.go` *(new)*, `internal/mcp/server.go` |

---

## Track A — CI Fixes (Issues #264 + #265)

*Single PR. Both changes are in `.github/workflows/ci.yml`.*

### Task 1: Fix golangci-lint-action version (#264)

**Files:**
- Modify: `.github/workflows/ci.yml` (lint job, line ~62)

- [ ] **Step 1: Update the action version**

In `.github/workflows/ci.yml`, change:
```yaml
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v2.11.4
```
to:
```yaml
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v7
        with:
          version: v2.11.4
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: upgrade golangci-lint-action to v7 — v6 rejects lint v2.x"
```

---

### Task 2: Fix postgres password in CI (#265)

**Files:**
- Modify: `.github/workflows/ci.yml` (test job, postgres service + TEST_DATABASE_URL env)

**Context:** `internal/db/postgres.go:rejectDefaultPassword()` blocks start-up when the DSN password is `"engram"` or `"postgres"`. The CI postgres service currently uses `POSTGRES_PASSWORD: engram`, triggering the guard for every test that touches the DB.

- [ ] **Step 1: Change the postgres service password and DSN**

In `.github/workflows/ci.yml`, find the `services.postgres.env` block and the `TEST_DATABASE_URL` env var, and update both:

```yaml
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: engram
          POSTGRES_PASSWORD: ci_test_secret
          POSTGRES_DB: engram_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
```

And update the test step env:
```yaml
      - name: Test with coverage
        env:
          TEST_DATABASE_URL: postgres://engram:ci_test_secret@localhost:5432/engram_test?sslmode=disable
        run: go test -coverprofile=coverage.out ./...
```

- [ ] **Step 2: Commit (amend the previous commit — same PR)**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: use non-default postgres password — security guard rejects 'engram'"
```

---

## Track B — Security Advisory Doc (Issue #261)

*Single PR.*

### Task 3: Add startup warning for ENGRAM_TRUST_PROXY_HEADERS (#261)

**Files:**
- Modify: `internal/mcp/server.go` (~line 119)
- Modify: `docs/operations.md` (~line 92)

**Context:** When `ENGRAM_TRUST_PROXY_HEADERS=1`, the server accepts `X-Forwarded-For` at face value. Any client that already has the Bearer token can also spoof their IP to bypass rate limiting.

- [ ] **Step 1: Add startup `slog.Warn` in server.go**

In `internal/mcp/server.go`, find the block at ~line 119 that reads `ENGRAM_TRUST_PROXY_HEADERS`. It currently looks like:

```go
if v := os.Getenv("ENGRAM_TRUST_PROXY_HEADERS"); v == "1" || strings.EqualFold(v, "true") {
    s.trustProxyHeaders = true
}
```

Change it to:

```go
if v := os.Getenv("ENGRAM_TRUST_PROXY_HEADERS"); v == "1" || strings.EqualFold(v, "true") {
    s.trustProxyHeaders = true
    slog.Warn("ENGRAM_TRUST_PROXY_HEADERS is enabled — ensure a trusted reverse proxy terminates all inbound connections; direct clients can spoof X-Forwarded-For to bypass rate limiting")
}
```

Make sure `log/slog` is in the import block (it likely already is; confirm with `grep '"log/slog"' internal/mcp/server.go`).

- [ ] **Step 2: Update docs/operations.md**

Find the `ENGRAM_TRUST_PROXY_HEADERS` entry (~line 92) and append the security note:

```markdown
**`ENGRAM_TRUST_PROXY_HEADERS`:** When Engram is behind a reverse proxy (nginx, Traefik, Caddy), set `ENGRAM_TRUST_PROXY_HEADERS=1` in `.env`. This enables the server to read the real client IP from `X-Forwarded-For` / `X-Real-IP` headers for rate limiting and `/setup-token` locality checks. Default is `false` — leave it off unless you have a trusted reverse proxy in front.

> **Security note:** Enabling this flag without a trusted L7 proxy in front allows any client with the Bearer token to supply an arbitrary `X-Forwarded-For` header, bypassing per-IP rate limiting. The Bearer token requirement still applies — this is not an authentication bypass.
```

- [ ] **Step 3: Build to confirm no compile errors**

```bash
go build ./...
```
Expected: clean exit, no output.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/server.go docs/operations.md
git commit -m "fix: warn at startup when ENGRAM_TRUST_PROXY_HEADERS is enabled (#261)"
```

---

## Track C — Python Consolidator Hardening (Issues #266 + #267)

*Single PR.*

### Task 4: Harden _get_config() error handling (#266)

**Files:**
- Modify: `consolidator/instinct/engram_client.py` (~line 21)
- Modify: `consolidator/tests/test_engram_client.py`

**Context:** `_get_config()` navigates `cfg["mcpServers"]["engram"]["headers"]["Authorization"]` with no guard. A missing key raises a bare `KeyError`; the Bearer token appears in tracebacks.

- [ ] **Step 1: Write failing tests first**

Add to `consolidator/tests/test_engram_client.py`:

```python
import json
import pytest
from pathlib import Path
from unittest.mock import patch, AsyncMock
from instinct.engram_client import EngramClient


def test_get_config_missing_file_raises_friendly_error(tmp_path):
    client = EngramClient.__new__(EngramClient)
    client._config_path = str(tmp_path / "nonexistent.json")
    with pytest.raises(RuntimeError, match="Engram MCP config not found"):
        client._get_config()


def test_get_config_missing_key_raises_friendly_error(tmp_path):
    cfg = tmp_path / "mcp_servers.json"
    cfg.write_text(json.dumps({"mcpServers": {}}))
    client = EngramClient.__new__(EngramClient)
    client._config_path = str(cfg)
    with pytest.raises(RuntimeError, match="Engram MCP server not configured"):
        client._get_config()


def test_get_config_does_not_leak_token_in_repr(tmp_path):
    cfg = tmp_path / "mcp_servers.json"
    cfg.write_text(json.dumps({
        "mcpServers": {
            "engram": {
                "url": "http://localhost:8788/sse",
                "headers": {"Authorization": "Bearer secret_token_xyz"}
            }
        }
    }))
    client = EngramClient.__new__(EngramClient)
    client._config_path = str(cfg)
    url, auth = client._get_config()
    assert url == "http://localhost:8788/sse"
    assert auth == "Bearer secret_token_xyz"
    # Confirm the method returns, not repr of the whole config
    assert "secret_token_xyz" not in repr(client)
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd consolidator && python -m pytest tests/test_engram_client.py::test_get_config_missing_file_raises_friendly_error tests/test_engram_client.py::test_get_config_missing_key_raises_friendly_error -v
```
Expected: FAIL (no RuntimeError raised yet).

- [ ] **Step 3: Rewrite _get_config() with proper error handling**

In `consolidator/instinct/engram_client.py`, replace:

```python
    def _get_config(self) -> tuple[str, str]:
        """Read SSE URL and Bearer token from mcp_servers.json."""
        cfg = json.loads(Path(self._config_path).read_text())
        engram = cfg["mcpServers"]["engram"]
        return engram["url"], engram["headers"]["Authorization"]
```

with:

```python
    def _get_config(self) -> tuple[str, str]:
        """Read SSE URL and Bearer token from mcp_servers.json."""
        config_path = Path(self._config_path)
        if not config_path.exists():
            raise RuntimeError(
                f"Engram MCP config not found at {config_path}. "
                "Run `claude mcp add engram` or create the file manually."
            )
        try:
            cfg = json.loads(config_path.read_text())
            engram = cfg["mcpServers"]["engram"]
            url = engram["url"]
            auth = engram["headers"]["Authorization"]
        except (KeyError, json.JSONDecodeError) as exc:
            raise RuntimeError(
                f"Engram MCP server not configured in {config_path}. "
                "Expected: mcpServers.engram.url and mcpServers.engram.headers.Authorization"
            ) from exc
        return url, auth
```

- [ ] **Step 4: Run all consolidator tests**

```bash
cd consolidator && python -m pytest tests/ -q
```
Expected: all pass (minimum 24 total, plus the 3 new tests = 27).

- [ ] **Step 5: Commit**

```bash
git add consolidator/instinct/engram_client.py consolidator/tests/test_engram_client.py
git commit -m "fix(consolidator): _get_config() raises friendly RuntimeError, no bare KeyError (#266)"
```

---

### Task 5: Log skipped lines in load_and_rotate_buffer (#267)

**Files:**
- Modify: `consolidator/instinct/run.py` (~line 17)
- Modify: `consolidator/tests/test_run.py`

**Context:** Corrupted or partially-written lines in the buffer are silently skipped after rotation — the file is already renamed, so the events are gone with no log output.

- [ ] **Step 1: Write the failing test**

Add to `consolidator/tests/test_run.py`:

```python
def test_load_and_rotate_logs_skipped_malformed_lines(tmp_path, capsys):
    buffer = tmp_path / "buffer.jsonl"
    good = make_events(19)
    with buffer.open("w") as f:
        for e in good:
            f.write(json.dumps(e) + "\n")
        f.write("this is not json\n")  # 1 malformed line → 20 total lines
    events = load_and_rotate_buffer(buffer)
    assert len(events) == 19
    assert not buffer.exists()  # still rotated
    out = capsys.readouterr().out
    assert "1 malformed" in out
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd consolidator && python -m pytest tests/test_run.py::test_load_and_rotate_logs_skipped_malformed_lines -v
```
Expected: FAIL — no "1 malformed" in output.

- [ ] **Step 3: Update load_and_rotate_buffer to count and log skipped lines**

In `consolidator/instinct/run.py`, replace the parse loop and rotation block:

```python
    events = []
    for line in raw_lines:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue

    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    dest = buffer_path.parent / f"buffer.jsonl.{ts}.processed"
    buffer_path.rename(dest)

    return events
```

with:

```python
    events = []
    skipped = 0
    for line in raw_lines:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            skipped += 1

    if skipped:
        print(f"instinct: WARN — {skipped} malformed line(s) skipped in {buffer_path.name}")

    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    dest = buffer_path.parent / f"buffer.jsonl.{ts}.processed"
    buffer_path.rename(dest)

    return events
```

- [ ] **Step 4: Run all consolidator tests**

```bash
cd consolidator && python -m pytest tests/ -q
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add consolidator/instinct/run.py consolidator/tests/test_run.py
git commit -m "fix(consolidator): log count of malformed lines skipped in load_and_rotate_buffer (#267)"
```

---

## Track D — memory_ingest_export MCP Tool (Issue #262)

*Prerequisite: `feat/ingest-parsers` must be merged before starting this track.*

### Task 6: Create PR for feat/ingest-parsers and merge it

**Context:** The `internal/ingest/slack`, `internal/ingest/router`, `internal/ingest/chatgpt`, `internal/ingest/claudeai` packages are on `feat/ingest-parsers`. They are already tested (13 Slack tests, 83.2% coverage). The new tool depends on `slack.ParseFile` and `router.ParseAuto`.

- [ ] **Step 1: Create the PR**

```bash
GITHUB_TOKEN="" gh pr create \
  --repo petersimmons1972/engram-go \
  --head feat/ingest-parsers \
  --base main \
  --title "feat(ingest): format router + Slack/ChatGPT/Claude.ai export parsers" \
  --body "Implements format-aware ingest for AI conversation exports.

- internal/ingest/claudeai: Claude.ai conversations.json parser
- internal/ingest/chatgpt: ChatGPT conversations.json parser
- internal/ingest/slack: Slack workspace .zip export parser (13 tests, 83.2% coverage)
- internal/ingest/router: format detection + ParseAuto dispatcher
- internal/mcp/ingest_document.go: wires router into memory_ingest_document_stream

Prerequisite for #262 (memory_ingest_export tool)."
```

- [ ] **Step 2: Merge the PR**

```bash
GITHUB_TOKEN="" gh pr merge <PR_NUMBER> --repo petersimmons1972/engram-go --merge --admin
```

- [ ] **Step 3: Pull main**

```bash
git checkout main && git pull
```

---

### Task 7: Extend router to support Slack zip detection

**Files:**
- Modify: `internal/ingest/router/router.go`
- Modify: `internal/ingest/router/router_test.go`

**Context:** `router.Detect` currently identifies Claude.ai and ChatGPT text exports. `memory_ingest_export` needs to route `.zip` files to the Slack parser. Adding `FormatSlack` to the router and a `ParseAutoFromPath` function keeps the routing logic in one place.

- [ ] **Step 1: Write failing tests**

Add to `internal/ingest/router/router_test.go`:

```go
func TestDetectFromPath_ZipExtension(t *testing.T) {
    f, err := os.CreateTemp(t.TempDir(), "export*.zip")
    if err != nil {
        t.Fatal(err)
    }
    f.Write([]byte{0x50, 0x4B, 0x03, 0x04}) // PK zip magic
    f.Close()

    got := DetectFromPath(f.Name())
    if got != FormatSlack {
        t.Errorf("want FormatSlack, got %q", got)
    }
}

func TestDetectFromPath_ZipMagicNoExtension(t *testing.T) {
    f, err := os.CreateTemp(t.TempDir(), "export")
    if err != nil {
        t.Fatal(err)
    }
    f.Write([]byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00})
    f.Close()

    got := DetectFromPath(f.Name())
    if got != FormatSlack {
        t.Errorf("want FormatSlack for zip magic, got %q", got)
    }
}

func TestDetectFromPath_TextFileUsesContentDetect(t *testing.T) {
    content := `[{"chat_messages":[]}]`
    f, err := os.CreateTemp(t.TempDir(), "claude*.json")
    if err != nil {
        t.Fatal(err)
    }
    f.WriteString(content)
    f.Close()

    got := DetectFromPath(f.Name())
    if got != FormatClaudeAI {
        t.Errorf("want FormatClaudeAI, got %q", got)
    }
}

func TestDetectFromPath_UnknownReturnsUnknown(t *testing.T) {
    f, err := os.CreateTemp(t.TempDir(), "random*.txt")
    if err != nil {
        t.Fatal(err)
    }
    f.WriteString("hello world")
    f.Close()

    got := DetectFromPath(f.Name())
    if got != FormatUnknown {
        t.Errorf("want FormatUnknown, got %q", got)
    }
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
go test ./internal/ingest/router/... -run TestDetectFromPath -v
```
Expected: FAIL — `DetectFromPath` not defined.

- [ ] **Step 3: Add FormatSlack and DetectFromPath to router.go**

In `internal/ingest/router/router.go`, add:

1. New format constant after `FormatChatGPT`:
```go
const (
    FormatUnknown  Format = "unknown"
    FormatClaudeAI Format = "claudeai"
    FormatChatGPT  Format = "chatgpt"
    FormatSlack    Format = "slack"
)
```

2. Add the `DetectFromPath` function after `ParseAuto`:
```go
// zipMagic is the four-byte PK header that identifies all ZIP archives.
var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// DetectFromPath opens the file at path, reads the first few bytes, and
// returns the detected format. Zip files (magic bytes PK\x03\x04) are
// classified as FormatSlack; everything else falls back to Detect on the
// first peekSize bytes.
func DetectFromPath(path string) Format {
    f, err := os.Open(path)
    if err != nil {
        return FormatUnknown
    }
    defer f.Close()

    buf := make([]byte, peekSize)
    n, _ := f.Read(buf)
    if n < 4 {
        return FormatUnknown
    }
    peek := buf[:n]
    if bytes.HasPrefix(peek, zipMagic) {
        return FormatSlack
    }
    return Detect(peek)
}
```

Add `"os"` to the import block in `router.go`.

- [ ] **Step 4: Run all router tests**

```bash
go test ./internal/ingest/router/... -v
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/router/router.go internal/ingest/router/router_test.go
git commit -m "feat(router): add FormatSlack and DetectFromPath for zip magic detection (#262)"
```

---

### Task 8: Implement memory_ingest_export handler

**Files:**
- Create: `internal/mcp/ingest_export.go`
- Create: `internal/mcp/ingest_export_test.go`
- Modify: `internal/mcp/server.go` (registration)

**Context:** The new tool accepts a server-local `path` and an optional `project`. It reads the file, routes via `router.DetectFromPath`, calls the appropriate parser, stamps memories with the project, and stores them. The handler follows the same `storeDocumentDeps` injection pattern as `ingest_document.go` for testability.

- [ ] **Step 1: Write failing tests**

Create `internal/mcp/ingest_export_test.go`:

```go
package mcp_test

import (
    "archive/zip"
    "bytes"
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"

    mcp "github.com/petersimmons1972/engram/internal/mcp"
    "github.com/petersimmons1972/engram/internal/testutil"
)

func TestHandleMemoryIngestExport_SlackZip(t *testing.T) {
    // Build a minimal valid Slack zip with one channel and one message.
    zipPath := buildMinimalSlackZip(t)

    pool, cleanup := testutil.NewTestEnginePool(t)
    defer cleanup()

    cfg := mcp.Config{DataDir: filepath.Dir(zipPath)}
    result, err := mcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, zipPath)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.MemoriesStored == 0 {
        t.Errorf("want at least 1 memory stored, got 0")
    }
    if result.Format != "slack" {
        t.Errorf("want format=slack, got %q", result.Format)
    }
}

func TestHandleMemoryIngestExport_ClaudeAIJSON(t *testing.T) {
    dir := t.TempDir()
    content := `[{"uuid":"abc","name":"Test","chat_messages":[{"uuid":"m1","sender":"human","text":"hello","created_at":"2026-01-01T00:00:00Z"}]}]`
    jsonPath := filepath.Join(dir, "conversations.json")
    os.WriteFile(jsonPath, []byte(content), 0o600)

    pool, cleanup := testutil.NewTestEnginePool(t)
    defer cleanup()

    cfg := mcp.Config{DataDir: dir}
    result, err := mcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, jsonPath)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.MemoriesStored != 1 {
        t.Errorf("want 1 memory, got %d", result.MemoriesStored)
    }
    if result.Format != "claudeai" {
        t.Errorf("want format=claudeai, got %q", result.Format)
    }
}

func TestHandleMemoryIngestExport_UnknownFormat_ReturnsError(t *testing.T) {
    dir := t.TempDir()
    txtPath := filepath.Join(dir, "random.txt")
    os.WriteFile(txtPath, []byte("not an export"), 0o600)

    pool, cleanup := testutil.NewTestEnginePool(t)
    defer cleanup()

    cfg := mcp.Config{DataDir: dir}
    _, err := mcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, txtPath)
    if err == nil {
        t.Error("want error for unknown format, got nil")
    }
}

func TestHandleMemoryIngestExport_PathOutsideDataDir_ReturnsError(t *testing.T) {
    pool, cleanup := testutil.NewTestEnginePool(t)
    defer cleanup()

    cfg := mcp.Config{DataDir: t.TempDir()}
    _, err := mcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, "/etc/passwd")
    if err == nil {
        t.Error("want path validation error, got nil")
    }
}

// buildMinimalSlackZip creates a valid Slack export zip with one channel.
func buildMinimalSlackZip(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "slack-export.zip")
    f, _ := os.Create(path)
    w := zip.NewWriter(f)

    writeZipFile := func(name, content string) {
        zf, _ := w.Create(name)
        zf.Write([]byte(content))
    }

    users := `[{"id":"U001","name":"alice","real_name":"Alice"}]`
    channels := `[{"id":"C001","name":"general"}]`
    msgs := `[{"type":"message","user":"U001","text":"Hello world","ts":"1700000000.000001"}]`

    writeZipFile("users.json", users)
    writeZipFile("channels.json", channels)
    writeZipFile("general/2026-01-01.json", msgs)
    w.Close()
    f.Close()
    return path
}

// IngestExportResult mirrors the JSON result shape for assertions.
type IngestExportResult struct {
    Format         string   `json:"format"`
    MemoriesStored int      `json:"memories_stored"`
    MemoryIDs      []string `json:"memory_ids"`
}
```

- [ ] **Step 2: Add the test helper to the export_test.go helpers file**

In `internal/mcp/export_test.go` (or create a new `ingest_export_helpers_test.go`), add the exported test helper:

```go
// CallHandleMemoryIngestExport is a test helper that invokes the
// memory_ingest_export handler and returns a parsed result.
func CallHandleMemoryIngestExport(
    ctx context.Context,
    t *testing.T,
    pool *EnginePool,
    project string,
    cfg Config,
    path string,
) (IngestExportResult, error) {
    t.Helper()
    req := newToolRequest(map[string]any{
        "path":    path,
        "project": project,
    })
    result, err := handleMemoryIngestExport(ctx, pool, req, cfg)
    if err != nil {
        return IngestExportResult{}, err
    }
    if result.IsError != nil && *result.IsError {
        return IngestExportResult{}, fmt.Errorf("tool error: %s", extractTextContent(result))
    }
    var out IngestExportResult
    json.Unmarshal([]byte(extractTextContent(result)), &out)
    return out, nil
}
```

*(Note: `newToolRequest` and `extractTextContent` are helpers already in the test file — confirm their names by checking `internal/mcp/export_test.go`.)*

- [ ] **Step 3: Run tests to confirm they fail**

```bash
go test ./internal/mcp/... -run TestHandleMemoryIngestExport -v
```
Expected: FAIL — `handleMemoryIngestExport` not defined.

- [ ] **Step 4: Implement the handler**

Create `internal/mcp/ingest_export.go`:

```go
package mcp

import (
    "context"
    "fmt"
    "os"

    mcpgo "github.com/mark3labs/mcp-go/mcp"
    "github.com/petersimmons1972/engram/internal/ingest/router"
    "github.com/petersimmons1972/engram/internal/ingest/slack"
    "github.com/petersimmons1972/engram/internal/types"
)

// handleMemoryIngestExport implements the memory_ingest_export MCP tool.
// It accepts a server-local file path, detects the export format (Slack zip,
// Claude.ai JSON, or ChatGPT JSON), parses the file, and stores one engram
// Memory per conversation / channel.
func handleMemoryIngestExport(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()

    project, err := getProject(args, "default")
    if err != nil {
        return nil, err
    }

    path := getString(args, "path", "")
    if path == "" {
        return mcpgo.NewToolResultError("path is required"), nil
    }

    safePath, err := SafePath(cfg.DataDir, path)
    if err != nil {
        return nil, fmt.Errorf("invalid path: %w", err)
    }

    format := router.DetectFromPath(safePath)
    if format == router.FormatUnknown {
        return nil, fmt.Errorf("memory_ingest_export: unrecognised format for %q — supported: slack (.zip), claudeai, chatgpt", path)
    }

    var memories []*types.Memory
    switch format {
    case router.FormatSlack:
        memories, err = slack.ParseFile(safePath)
        if err != nil {
            return nil, fmt.Errorf("memory_ingest_export: slack parse: %w", err)
        }
    default:
        // Claude.ai or ChatGPT: read as text and use router.ParseAuto.
        f, openErr := os.Open(safePath)
        if openErr != nil {
            return nil, fmt.Errorf("memory_ingest_export: open: %w", openErr)
        }
        defer f.Close()
        _, memories, err = router.ParseAuto(f)
        if err != nil {
            return nil, fmt.Errorf("memory_ingest_export: parse: %w", err)
        }
    }

    h, err := pool.Get(ctx, project)
    if err != nil {
        return nil, err
    }
    deps := storeDocumentDeps{
        engine:  engineStorerAdapter{store: h.Engine.StoreWithRawBody},
        backend: backendDocumentAdapter{b: h.Engine.Backend()},
    }
    return runExportFanout(ctx, deps, project, format, memories)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/mcp/... -run TestHandleMemoryIngestExport -v
```
Expected: all 4 tests pass.

- [ ] **Step 6: Commit handler**

```bash
git add internal/mcp/ingest_export.go internal/mcp/ingest_export_test.go
git commit -m "feat(mcp): implement memory_ingest_export handler for Slack zip + text exports (#262)"
```

---

### Task 9: Register memory_ingest_export in server.go

**Files:**
- Modify: `internal/mcp/server.go`

- [ ] **Step 1: Add the tool to the registration list**

In `internal/mcp/server.go`, find the `tools` slice in `registerTools()` (~line 451). Add an entry:

```go
{"memory_ingest_export",
    "Ingest a server-local AI conversation export file (Slack workspace .zip, Claude.ai conversations.json, or ChatGPT conversations.json). Parses the file, auto-detects format, and stores one memory per conversation or channel.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryIngestExport(ctx, pool, req, cfg)
    }},
```

- [ ] **Step 2: Build**

```bash
go build ./...
```
Expected: clean.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -count=1 -race 2>&1 | grep -E "^(ok|FAIL)"
```
Expected: all `ok`.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/server.go
git commit -m "feat(mcp): register memory_ingest_export tool (#262)"
```

---

## Verification

After all tracks land:

```bash
# Go — all packages
go test ./... -count=1 -race 2>&1 | grep -E "^(ok|FAIL)"

# Python consolidator
cd consolidator && python -m pytest tests/ -q

# Confirm tool is visible in MCP
go run ./cmd/engram --list-tools 2>/dev/null | grep ingest_export || echo "check server startup log"

# Confirm CI passes (after PRs merge)
GITHUB_TOKEN="" gh run list --repo petersimmons1972/engram-go --branch main --limit 3 --json conclusion,headBranch
```

---

## PR Plan

| Track | Issues | Branch name |
|-------|--------|-------------|
| A | #264, #265 | `fix/ci-lint-and-postgres` |
| B | #261 | `fix/proxy-headers-warn` |
| C | #266, #267 | `fix/consolidator-hardening` |
| D | #262 | `feat/ingest-export-tool` (after `feat/ingest-parsers` merges) |
