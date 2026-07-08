#!/usr/bin/env bash
# Verifies that ops/instinct.service, the Makefile's install-instinct target, and
# the instinct hooks all agree on the same consolidator binary name/path — and,
# critically, that this is proven by actually building the binary rather than
# just string-matching. A previous version of this test only grepped for the
# expected name in the unit file, which caught nothing when hooks/install.sh
# and hooks/post-tool-use.sh silently drifted to a different binary name.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

unit="ops/instinct.service"
makefile="Makefile"

fail() {
    echo "FAIL: $1" >&2
    exit 1
}

[[ -f "$unit" ]] || fail "missing $unit"

exec_start_line="$(grep -E '^ExecStart=' "$unit" || true)"
[[ -n "$exec_start_line" ]] || fail "missing ExecStart in $unit"

expected='ExecStart=%h/bin/instinct-consolidate'
[[ "$exec_start_line" == "$expected" ]] || fail "expected $expected, got: $exec_start_line"

if grep -Eq '^Environment=PYTHONPATH=' "$unit"; then
    fail "unexpected PYTHONPATH environment in $unit"
fi

if grep -Fq 'python3 -m instinct.run' "$unit"; then
    fail "unexpected removed python module entrypoint in $unit"
fi

# Derive the expected binary basename directly from the unit's ExecStart line
# (rather than hardcoding it a second time) so this can't itself drift from the
# check above.
binary_name="$(basename "${exec_start_line#ExecStart=%h/bin/}")"
[[ -n "$binary_name" ]] || fail "could not derive binary name from ExecStart in $unit"

# --- Cross-check 1: hooks/install.sh and hooks/post-tool-use.sh reference the
# --- same binary name (blocker: these previously drifted to "instinct").
grep -Fq "$binary_name" hooks/install.sh ||
    fail "hooks/install.sh does not build/reference $binary_name (name drift vs $unit)"
grep -Fq "$binary_name" hooks/post-tool-use.sh ||
    fail "hooks/post-tool-use.sh does not check for $binary_name on PATH (name drift vs $unit)"

# --- Cross-check 2: actually BUILD the binary via `make install-instinct`
# (the unit file's own header comment names this as the install path) into a
# throwaway HOME, and confirm the artifact lands at the exact name/path this
# unit's ExecStart expects. This catches drift even if someone renames the
# Makefile's build target without touching this test's string checks.
[[ -f "$makefile" ]] || fail "missing $makefile"
grep -Eq '^install-instinct:' "$makefile" ||
    fail "$makefile has no install-instinct target (referenced by $unit's header comment)"

command -v go >/dev/null 2>&1 || fail "go toolchain not found — cannot verify install-instinct build"

tmp_home="$(mktemp -d)"
trap 'rm -rf "$tmp_home"' EXIT
mkdir -p "$tmp_home/bin"

# Preserve the real Go toolchain/module cache locations — only the install
# destination ($HOME/bin) needs to be redirected into the throwaway dir.
# Overriding HOME wholesale (without this) makes GOTOOLCHAIN=auto try to
# locate/download the pinned toolchain under the fake HOME/sdk instead of
# reusing the one already on disk at the real $HOME/sdk.
if [[ -d "$HOME/sdk" ]]; then
    ln -s "$HOME/sdk" "$tmp_home/sdk"
fi

build_log="$(mktemp)"
# -buildvcs=false: under the throwaway HOME, git's safe.directory allowlist
# (read from HOME) no longer trusts this checkout, which would otherwise make
# `go build` fail trying to stamp VCS info it can't read.
if ! env HOME="$tmp_home" \
        GOPATH="$(go env GOPATH)" \
        GOCACHE="$(go env GOCACHE)" \
        GOROOT="$(go env GOROOT)" \
        GOFLAGS="-buildvcs=false" \
        make install-instinct >"$build_log" 2>&1; then
    cat "$build_log" >&2
    rm -f "$build_log"
    fail "make install-instinct failed to build"
fi
rm -f "$build_log"

built_path="$tmp_home/bin/$binary_name"
[[ -x "$built_path" ]] ||
    fail "make install-instinct did not produce an executable at $built_path — Makefile output name has drifted from $unit's ExecStart"

echo "PASS: $unit uses the installed Go consolidator contract"
echo "PASS: hooks/install.sh and hooks/post-tool-use.sh agree on binary name '$binary_name'"
echo "PASS: 'make install-instinct' builds an executable matching $unit's ExecStart ($built_path)"
