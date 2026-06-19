#!/usr/bin/env python3
"""Capture CC synthesized 'user rejected' tool denials to denial-log.md.

Reads PostToolUse JSON from stdin. Argv[1] is the path to denial-log.md.
"""

import datetime
import fcntl
import json
import os
import sys
import tempfile
import urllib.request

DENIAL_MARKERS = (
    "User rejected tool use",
    "user doesn't want to proceed with this tool use",
    "tool use was rejected",
)

MAX_ENTRIES = 100
HEALTH_PROBE_TIMEOUT = 2  # seconds


def _resolve_engram_base_url() -> str:
    """Priority: env var > engram-endpoint.conf > localhost fallback."""
    from_env = os.environ.get("ENGRAM_BASE_URL", "").strip()
    if from_env:
        return from_env
    conf = os.path.expanduser("~/.claude/hooks/engram-endpoint.conf")
    try:
        with open(conf) as f:
            for line in f:
                line = line.strip()
                if line.startswith("ENGRAM_BASE_URL="):
                    val = line.split("=", 1)[1].strip().strip('"').strip("'")
                    if val:
                        return val
    except Exception:
        pass
    return "http://127.0.0.1:8788"


HEALTH_PROBE_URL = f"{_resolve_engram_base_url().rstrip('/')}/health"


def is_synthesized_denial(payload: dict) -> bool:
    resp = payload.get("tool_response", "")
    if isinstance(resp, dict):
        resp = json.dumps(resp)
    return any(m in str(resp) for m in DENIAL_MARKERS)


def summarize_input(inp) -> dict:
    if not isinstance(inp, dict):
        return {}
    out = {}
    for k in ("project", "memory_type", "tags", "importance", "query", "limit", "id"):
        if k in inp:
            v = inp[k]
            if isinstance(v, str) and len(v) > 200:
                v = v[:200] + "...(truncated)"
            out[k] = v
    if "content" in inp:
        c = inp["content"]
        out["content_len"] = len(c) if isinstance(c, str) else None
    return out


def probe_engram_health() -> dict:
    """GET HEALTH_PROBE_URL via urllib; fail-open on any error/timeout/non-JSON."""
    try:
        req = urllib.request.urlopen(  # noqa: S310
            HEALTH_PROBE_URL, timeout=HEALTH_PROBE_TIMEOUT
        )
        body = req.read().decode("utf-8", errors="replace")
        return json.loads(body)
    except Exception:
        pass
    return {"probe_error": "unreachable"}


def append_record_bounded(path: str, record: str, cap: int = MAX_ENTRIES) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    lock_path = path + ".lock"
    with open(lock_path, "w") as lf:
        fcntl.flock(lf, fcntl.LOCK_EX)

        try:
            existing = open(path).read()
        except FileNotFoundError:
            existing = "# Engram MCP synthesized-denial log\n\nOne JSON record per line.\n\n"

        lines = existing.splitlines()
        json_lines = [l for l in lines if l.startswith("{")]
        header_lines = [l for l in lines if not l.startswith("{")]

        if len(json_lines) >= cap:
            json_lines = json_lines[-(cap - 1):]
            new_content = "\n".join(header_lines).rstrip() + "\n\n" + "\n".join(json_lines + [record]) + "\n"
        else:
            new_content = existing.rstrip() + "\n" + record + "\n"

        dir_ = os.path.dirname(path)
        fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".denial_log_tmp")
        try:
            with os.fdopen(fd, "w") as f:
                f.write(new_content)
            os.replace(tmp, path)
        except Exception:
            try:
                os.unlink(tmp)
            except FileNotFoundError:
                pass
            raise


def main() -> int:
    if len(sys.argv) < 2:
        return 0
    path = sys.argv[1]

    try:
        payload = json.loads(sys.stdin.read())
    except Exception:
        return 0

    if not is_synthesized_denial(payload):
        return 0

    record = {
        "ts": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "kind": "tool_synthesized_denial",
        "tool_name": payload.get("tool_name", "unknown"),
        "tool_use_id": payload.get("tool_use_id", ""),
        "session_id": payload.get("session_id") or payload.get("sessionId") or "",
        "tool_input_summary": summarize_input(payload.get("tool_input")),
        "engram_health_at_event": probe_engram_health(),
    }
    append_record_bounded(path, json.dumps(record, separators=(",", ":")))

    # systemMessage so Claude knows the capture happened.
    print(json.dumps({"systemMessage": "Synthesized tool denial captured to denial-log.md. Likely upstream timeout, not user action."}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
