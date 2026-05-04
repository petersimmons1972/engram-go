#!/usr/bin/env python3
"""
Configurable mock HTTP server for hook pipeline tests.
Port: $MOCK_PORT env var (default 19788).
Control plane:
  POST /__configure  {"endpoint": "/quick-store", "mode": "422", "after_n": 0, "delay": 0}
  POST /__reset      clears config and request log
  GET  /__requests   returns [{method, path, body, ts}, ...]
Modes: "200", "422", "503", "slow:<seconds>", "timeout", "reset_after_n:<n>"
"""
import json
import os
import sys
import time
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from datetime import datetime, timezone

PORT = int(os.environ.get("MOCK_PORT", 19788))

_lock = threading.Lock()
_config: dict = {}   # endpoint -> {mode, after_n, delay, hit_count}
_requests: list = []


def _ts():
    return datetime.now(timezone.utc).isoformat()


def _get_cfg(path):
    with _lock:
        return _config.get(path, {}).copy()


def _record(method, path, body):
    with _lock:
        _requests.append({"method": method, "path": path, "body": body, "ts": _ts()})


def _requests_for(path):
    with _lock:
        return [r for r in _requests if r["path"] == path]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass  # suppress access log

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        return self.rfile.read(length).decode() if length else ""

    def _send(self, status, body=""):
        encoded = body.encode() if isinstance(body, str) else body
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self):
        if self.path == "/__requests":
            with _lock:
                self._send(200, json.dumps(_requests))
            return
        if self.path.startswith("/__requests/"):
            endpoint = self.path[len("/__requests"):]
            self._send(200, json.dumps(_requests_for(endpoint)))
            return
        _record("GET", self.path, "")
        cfg = _get_cfg(self.path)
        if not cfg and self.path not in ("/__requests", "/__reset"):
            self._send(404, json.dumps({"error": "not found"}))
            return
        self._apply_cfg(self.path, "GET", "", cfg)

    def do_POST(self):
        body = self._read_body()

        if self.path == "/__configure":
            data = json.loads(body) if body else {}
            endpoint = data.get("endpoint", "/")
            with _lock:
                _config[endpoint] = {
                    "mode": data.get("mode", "200"),
                    "after_n": int(data.get("after_n", 0)),
                    "delay": float(data.get("delay", 0)),
                    "hit_count": 0,
                }
            self._send(200, '{"ok":true}')
            return

        if self.path == "/__reset":
            with _lock:
                _config.clear()
                _requests.clear()
            self._send(200, '{"ok":true}')
            return

        _record("POST", self.path, body)
        cfg = _get_cfg(self.path)
        self._apply_cfg(self.path, "POST", body, cfg)

    def _apply_cfg(self, path, method, body, cfg):
        mode = cfg.get("mode", "200")
        delay = cfg.get("delay", 0)
        after_n = cfg.get("after_n", 0)

        with _lock:
            if path in _config:
                _config[path]["hit_count"] += 1
                hit = _config[path]["hit_count"]
            else:
                hit = 1

        # after_n: succeed for first N requests, then use mode
        if after_n > 0 and hit <= after_n:
            mode = "200"

        if delay:
            time.sleep(delay)

        if mode == "timeout":
            # Accept connection, never respond — caller will time out
            time.sleep(120)
            return

        if mode == "200":
            # Special-case token endpoint response shape
            if "setup-token" in self.path or "token" in mode:
                self._send(200, json.dumps({"token": "test-token-mock", "endpoint": f"http://127.0.0.1:{PORT}/sse"}))
            else:
                self._send(200, json.dumps({"ok": True, "id": f"mock-{hit}"}))
        elif mode == "token":
            self._send(200, json.dumps({"token": "test-token-mock", "endpoint": f"http://127.0.0.1:{PORT}/sse"}))
        elif mode.isdigit():
            status = int(mode)
            self._send(status, json.dumps({"error": f"mock {status}", "code": status}))
        elif mode.startswith("slow:"):
            secs = float(mode.split(":")[1])
            time.sleep(secs)
            self._send(200, json.dumps({"ok": True}))
        else:
            self._send(200, json.dumps({"ok": True}))


def main():
    server = HTTPServer(("127.0.0.1", PORT), Handler)
    # Write PID and port for test harness
    pid_file = os.environ.get("MOCK_PID_FILE", f"/tmp/mock-server-{PORT}.pid")
    with open(pid_file, "w") as f:
        f.write(str(os.getpid()))
    print(f"mock-server listening on {PORT}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        if os.path.exists(pid_file):
            os.unlink(pid_file)


if __name__ == "__main__":
    main()
