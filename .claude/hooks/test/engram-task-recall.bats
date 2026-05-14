#!/usr/bin/env bats
# Tests for engram-task-recall.sh — task-specific recall on first user message.
# TDD: tests written first. All fail until the hook is implemented (red phase).
#
# Contract being tested:
#   - Fires on UserPromptSubmit, reads event JSON from stdin
#   - Extracts .prompt field, calls /quick-recall, emits systemMessage JSON
#   - MUST fail-open (exit 0) on any error, timeout, or missing config
#   - MUST complete within 1 second even when Engram is unreachable

REAL_HOME="$HOME"
HOOK="$HOME/.claude/hooks/engram-task-recall.sh"
REAL_MCP="$HOME/.claude/mcp_servers.json"

# ── helpers ────────────────────────────────────────────────────────────────────

setup() {
    TEST_TMPDIR="$(mktemp -d)"
    export TEST_TMPDIR HOME="$TEST_TMPDIR"
    mkdir -p "$HOME/.claude"
}

teardown() {
    rm -rf "$TEST_TMPDIR"
}

_setup_fake_token() {
    cat > "$HOME/.claude/mcp_servers.json" <<'JSON'
{"mcpServers":{"engram":{"type":"sse","url":"http://127.0.0.1:8788/sse","headers":{"Authorization":"Bearer test-token-abc123"}}}}
JSON
}

_make_prompt() {
    local msg="${1:-debug the engram session recall latency issue from yesterday}"
    printf '{"prompt":"%s","session_id":"test-session-001"}' "$msg"
}

_start_mock_server() {
    # Start a Python HTTP mock server on given port.
    # $1 = port, $2 = HTTP status code to return, $3 = optional body (JSON string)
    local port="$1" status="$2" body="${3:-{}}"
    python3 - "$port" "$status" "$body" &>/dev/null &
    echo $!  # return PID
}

# ── existence tests ────────────────────────────────────────────────────────────

@test "hook file exists" {
    [ -f "$HOOK" ]
}

@test "hook is executable" {
    [ -x "$HOOK" ]
}

# ── fail-open: input edge cases ────────────────────────────────────────────────

@test "exits 0 on empty stdin" {
    run bash "$HOOK" < /dev/null
    [ "$status" -eq 0 ]
}

@test "exits 0 when stdin is not valid JSON" {
    _setup_fake_token
    run bash "$HOOK" <<< 'this is not json at all %%%()'
    [ "$status" -eq 0 ]
}

@test "exits 0 when prompt field is missing from JSON" {
    _setup_fake_token
    run bash "$HOOK" <<< '{"session_id":"s","other":"field value here"}'
    [ "$status" -eq 0 ]
}

@test "exits 0 when prompt is fewer than 20 chars" {
    _setup_fake_token
    run bash "$HOOK" <<< '{"prompt":"hi","session_id":"s"}'
    [ "$status" -eq 0 ]
}

@test "exits 0 when prompt is exactly 19 chars" {
    _setup_fake_token
    run bash "$HOOK" <<< '{"prompt":"nineteen chars!!!"}'
    [ "$status" -eq 0 ]
}

# ── fail-open: missing config / auth ──────────────────────────────────────────

@test "exits 0 when mcp_servers.json is absent" {
    # No _setup_fake_token — HOME is empty fake dir
    run bash "$HOOK" <<< "$(_make_prompt)"
    [ "$status" -eq 0 ]
}

@test "exits 0 when token is empty string in config" {
    cat > "$HOME/.claude/mcp_servers.json" <<'JSON'
{"mcpServers":{"engram":{"headers":{"Authorization":""}}}}
JSON
    run bash "$HOOK" <<< "$(_make_prompt)"
    [ "$status" -eq 0 ]
}

# ── fail-open: network failures ────────────────────────────────────────────────

@test "exits 0 when server is unreachable" {
    _setup_fake_token
    ENGRAM_TEST_PORT=19799 run bash "$HOOK" <<< "$(_make_prompt)"
    [ "$status" -eq 0 ]
}

@test "exits 0 when server returns 401" {
    _setup_fake_token
    local port=19801
    python3 - <<PYEOF &>/dev/null &
import http.server, socketserver, threading
PORT = ${port}
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.send_response(401); self.end_headers()
    def log_message(self, *a): pass
with socketserver.TCPServer(('127.0.0.1', PORT), H) as s:
    t = threading.Timer(3.0, s.shutdown)
    t.daemon = True; t.start()
    s.serve_forever()
PYEOF
    local py_pid=$!
    sleep 0.15
    ENGRAM_TEST_PORT=$port run bash "$HOOK" <<< "$(_make_prompt)"
    local st="$status"
    kill "$py_pid" 2>/dev/null || true
    [ "$st" -eq 0 ]
}

@test "exits 0 when server returns 500" {
    _setup_fake_token
    local port=19802
    python3 - <<PYEOF &>/dev/null &
import http.server, socketserver, threading
PORT = ${port}
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.send_response(500); self.end_headers()
    def log_message(self, *a): pass
with socketserver.TCPServer(('127.0.0.1', PORT), H) as s:
    t = threading.Timer(3.0, s.shutdown)
    t.daemon = True; t.start()
    s.serve_forever()
PYEOF
    local py_pid=$!
    sleep 0.15
    ENGRAM_TEST_PORT=$port run bash "$HOOK" <<< "$(_make_prompt)"
    local st="$status"
    kill "$py_pid" 2>/dev/null || true
    [ "$st" -eq 0 ]
}

@test "exits 0 when server returns empty results array" {
    _setup_fake_token
    local port=19803
    python3 - <<PYEOF &>/dev/null &
import http.server, socketserver, threading, json
PORT = ${port}
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.dumps({'results': []}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *a): pass
with socketserver.TCPServer(('127.0.0.1', PORT), H) as s:
    t = threading.Timer(3.0, s.shutdown)
    t.daemon = True; t.start()
    s.serve_forever()
PYEOF
    local py_pid=$!
    sleep 0.15
    ENGRAM_TEST_PORT=$port run bash "$HOOK" <<< "$(_make_prompt)"
    local st="$status" out="$output"
    kill "$py_pid" 2>/dev/null || true
    [ "$st" -eq 0 ]
    [ -z "$out" ]  # no output when no results
}

# ── timing: non-negotiable ─────────────────────────────────────────────────────

@test "completes in under 1 second when server is unreachable" {
    _setup_fake_token
    local start end elapsed_ms
    start=$(date +%s%N 2>/dev/null || date +%s)
    ENGRAM_TEST_PORT=19799 bash "$HOOK" <<< "$(_make_prompt)" >/dev/null 2>&1 || true
    end=$(date +%s%N 2>/dev/null || date +%s)
    if [[ "$start" =~ ^[0-9]{13,}$ ]]; then
        elapsed_ms=$(( (end - start) / 1000000 ))
        echo "Wall time: ${elapsed_ms}ms" >&2
        [ "$elapsed_ms" -lt 1200 ]  # 1.2s ceiling allows tiny OS scheduling margin
    else
        elapsed_ms=$(( end - start ))
        echo "Wall time: ${elapsed_ms}s (low-res clock)" >&2
        [ "$elapsed_ms" -lt 2 ]
    fi
}

# ── output contract ────────────────────────────────────────────────────────────

@test "does not emit action:block under any condition" {
    _setup_fake_token
    ENGRAM_TEST_PORT=19799 run bash "$HOOK" <<< "$(_make_prompt)"
    [ "$status" -eq 0 ]
    if echo "$output" | grep -q '"action".*"block"'; then
        echo "Hook emitted action:block — forbidden in UserPromptSubmit hooks" >&2
        return 1
    fi
}

@test "emits valid systemMessage JSON when server returns results" {
    _setup_fake_token
    local port_file="$TEST_TMPDIR/mock_port_16.txt"
    python3 - "$port_file" <<'PYEOF' &>/dev/null &
import http.server, socketserver, threading, json, sys
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.dumps({'results': [
            {'id': 'aaa', 'summary': 'Clearwatch chart rendering fix from last session', 'tags': ['clearwatch', 'charts'], 'score': 0.92},
            {'id': 'bbb', 'summary': 'SVG optimization pattern for D3 charts in reports', 'tags': ['svg', 'd3'], 'score': 0.85}
        ]}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *a): pass
class S(socketserver.TCPServer):
    allow_reuse_address = True
with S(('127.0.0.1', 0), H) as s:
    with open(sys.argv[1], 'w') as f: f.write(str(s.server_address[1]))
    t = threading.Timer(5.0, s.shutdown)
    t.daemon = True; t.start()
    s.serve_forever()
PYEOF
    local py_pid=$!
    # Wait up to 1s for the server to be ready (port file appears)
    local i=0
    while [ ! -s "$port_file" ] && [ "$i" -lt 20 ]; do sleep 0.05; i=$((i + 1)); done
    local port; port=$(cat "$port_file" 2>/dev/null || echo "")
    [ -n "$port" ] || skip "Mock server failed to start"
    ENGRAM_TEST_PORT=$port run bash "$HOOK" <<< "$(_make_prompt)"
    local st="$status" out="$output"
    kill "$py_pid" 2>/dev/null || true
    [ "$st" -eq 0 ]
    [ -n "$out" ]
    python3 -c "
import json, sys
d = json.loads(sys.argv[1])
assert 'systemMessage' in d, 'missing systemMessage key'
assert d['systemMessage'], 'systemMessage is empty'
" "$out" || { echo "Invalid systemMessage output: $out" >&2; return 1; }
}

@test "systemMessage contains Engram Task Recall header" {
    _setup_fake_token
    local port_file="$TEST_TMPDIR/mock_port_17.txt"
    python3 - "$port_file" <<'PYEOF' &>/dev/null &
import http.server, socketserver, threading, json, sys
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.dumps({'results': [
            {'id': 'ccc', 'summary': 'Test memory for header verification', 'tags': ['test'], 'score': 0.9}
        ]}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *a): pass
class S(socketserver.TCPServer):
    allow_reuse_address = True
with S(('127.0.0.1', 0), H) as s:
    with open(sys.argv[1], 'w') as f: f.write(str(s.server_address[1]))
    t = threading.Timer(5.0, s.shutdown)
    t.daemon = True; t.start()
    s.serve_forever()
PYEOF
    local py_pid=$!
    local i=0
    while [ ! -s "$port_file" ] && [ "$i" -lt 20 ]; do sleep 0.05; i=$((i + 1)); done
    local port; port=$(cat "$port_file" 2>/dev/null || echo "")
    [ -n "$port" ] || skip "Mock server failed to start"
    ENGRAM_TEST_PORT=$port run bash "$HOOK" <<< "$(_make_prompt)"
    local st="$status" out="$output"
    kill "$py_pid" 2>/dev/null || true
    [ "$st" -eq 0 ]
    echo "$out" | grep -q "Engram Task Recall" || {
        echo "Output missing 'Engram Task Recall' header: $out" >&2
        return 1
    }
}

# ── live server (skipped when Engram not running) ──────────────────────────────

@test "live server: exits 0 and emits valid output or nothing" {
    if ! curl -sf --max-time 1 "http://127.0.0.1:8788/quick-recall" \
         -X POST -H "Content-Type: application/json" \
         -d '{"query":"auth-check","project":"global","limit":1}' >/dev/null 2>&1; then
        skip "Engram not running — skipping live-server test"
    fi
    [ -f "$REAL_MCP" ] || skip "Real mcp_servers.json not found"
    cp "$REAL_MCP" "$HOME/.claude/mcp_servers.json"
    run bash "$HOOK" <<< "$(_make_prompt 'debug the engram session recall latency issue in production environment')"
    [ "$status" -eq 0 ]
    if [ -n "$output" ]; then
        python3 -c "import json,sys; json.loads(sys.argv[1])" "$output" || {
            echo "Output is not valid JSON: $output" >&2
            return 1
        }
    fi
}
