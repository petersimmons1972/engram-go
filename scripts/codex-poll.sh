#!/usr/bin/env bash
# codex-poll.sh — Claude↔Codex tiered work-loop poller (interim, superseded by codex#25)
#
# TIERED CADENCE (driven by a single 5-min systemd timer + tick counter):
#   Fast path  (every wake / 5 min): ONE cross-repo `gh search issues` for
#       agent/codex/queued. One API call, ZERO LLM tokens when empty. On a
#       hit: claim + run Codex (worktree off origin/main, skip dirty, flock,
#       one-per-wake).
#   Slow path  (every 6th wake / 30 min): sweep agent/codex/needs-input +
#       agent/codex/blocked for stale items. LOG/REPORT ONLY — never auto-acts.
#   Daily      (once per UTC day): git-pull ~/.cache/claude-codex to refresh
#       config/target-repos.txt + protocol docs.
#
# Kill switch (applies to ALL paths): touch ~/.codex-poll.disabled
#   pause:  touch ~/.codex-poll.disabled
#   resume: rm    ~/.codex-poll.disabled
#
# Manual single-path invocation for testing:
#   codex-poll.sh fast | codex-poll.sh slow | codex-poll.sh daily

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
CACHE_DIR="${CLAUDE_CODEX_CACHE:-${HOME}/.cache/claude-codex}"
export TARGET_REPOS_FILE="${CACHE_DIR}/config/target-repos.txt"
GITHUB_OWNER="${GITHUB_OWNER:-petersimmons1972}"
PROJECTS_ROOT="${PROJECTS_ROOT:-${HOME}/projects}"
WORKTREES_ROOT="${WORKTREES_ROOT:-${HOME}/projects/.codex-poll-worktrees}"
STATE_DIR="${HOME}/.local/state/codex-poll"
LOG_FILE="${STATE_DIR}/poll.log"
LOCK_FILE="${STATE_DIR}/poll.lock"
TICK_FILE="${STATE_DIR}/tick"
DAILY_STAMP="${STATE_DIR}/last-daily"
KILL_SWITCH="${HOME}/.codex-poll.disabled"
HANDOFF_BIN="${HOME}/bin/codex-handoff"
SLOW_EVERY="${SLOW_EVERY:-6}"      # every Nth wake -> slow path (6 * 5min = 30min)

mkdir -p "${STATE_DIR}" "${WORKTREES_ROOT}"

log() {
  local ts; ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  printf '[%s] %s\n' "${ts}" "$*" | tee -a "${LOG_FILE}"
}

repo_short_name() { printf '%s' "$1" | sed 's|^[^/]*/||'; }

# ---------------------------------------------------------------------------
# Embedded Python helpers (single-quoted bodies; config path via env var;
# all internal string literals use single quotes — no backslash escaping)
# ---------------------------------------------------------------------------

# Reads gh-search JSON on stdin, prints ALL candidates in priority order as: PRANK\tCREATED\tSLUG\tNUM
# (one line per candidate; caller iterates and picks the first eligible one)
PY_PICK_QUEUED='
import sys, json, os
rows = json.load(sys.stdin)
allowed = set()
with open(os.environ["TARGET_REPOS_FILE"]) as fh:
    for line in fh:
        s = line.split("#")[0].strip()
        if s:
            allowed.add(s)
prio = ("priority/p0", "priority/p1", "priority/p2", "priority/p3")
def prank(labels):
    names = {x["name"] for x in labels}
    for i, p in enumerate(prio):
        if p in names:
            return i
    return 9
cands = []
for r in rows:
    slug = r["repository"]["nameWithOwner"]
    if slug not in allowed:
        continue
    label_names = {x["name"] for x in r.get("labels", [])}
    # Coexistence race guard: skip issues already claimed by another worker,
    # and skip any that lost the queued label between search and pick.
    # (gh search CLI cannot express negative label qualifiers, so filter here.)
    if "agent/codex/in-progress" in label_names:
        continue
    if "agent/codex/queued" not in label_names:
        continue
    cands.append((prank(r.get("labels", [])), r["createdAt"], slug, r["number"]))
if not cands:
    sys.exit(0)
cands.sort(key=lambda c: (c[0], c[1]))
# Emit ALL candidates — caller iterates to find first eligible (has local checkout, clean tree)
for p, created, slug, num in cands:
    sys.stdout.write("\t".join([str(p), created, slug, str(num)]) + "\n")
'

# Reads gh-search JSON on stdin, prints one report line per config-repo item
PY_REPORT_STALE='
import sys, json, os, datetime
rows = json.load(sys.stdin)
allowed = set()
with open(os.environ["TARGET_REPOS_FILE"]) as fh:
    for line in fh:
        s = line.split("#")[0].strip()
        if s:
            allowed.add(s)
now = datetime.datetime.now(datetime.timezone.utc)
for r in rows:
    slug = r["repository"]["nameWithOwner"]
    if slug not in allowed:
        continue
    upd = datetime.datetime.fromisoformat(r["updatedAt"].replace("Z", "+00:00"))
    age_h = (now - upd).total_seconds() / 3600.0
    flag = "STALE" if age_h >= 24 else "ok"
    title = r["title"][:60]
    sys.stdout.write("slow:   {}#{} [{} {:.1f}h] {}\n".format(slug, r["number"], flag, age_h, title))
'

# ---------------------------------------------------------------------------
# DAILY: refresh the protocol/config cache via git pull
# ---------------------------------------------------------------------------
daily_path() {
  local today; today="$(date -u +%Y-%m-%d)"
  if [[ -f "${DAILY_STAMP}" && "$(cat "${DAILY_STAMP}" 2>/dev/null)" == "${today}" ]]; then
    return 0
  fi
  if [[ -d "${CACHE_DIR}/.git" ]]; then
    log "daily: git pull cache ${CACHE_DIR}"
    if git -C "${CACHE_DIR}" pull --ff-only >>"${LOG_FILE}" 2>&1; then
      log "daily: cache refreshed"
    else
      log "daily: WARNING cache pull failed"
    fi
  else
    log "daily: cache is not a git checkout (${CACHE_DIR}); skipping pull"
  fi
  printf '%s\n' "${today}" > "${DAILY_STAMP}"
}

# ---------------------------------------------------------------------------
# SLOW PATH: report stale needs-input / blocked items (LOG ONLY, never acts)
# ---------------------------------------------------------------------------
slow_path() {
  log "slow: sweeping needs-input + blocked (report only)"
  local label rows count line
  for label in "agent/codex/needs-input" "agent/codex/blocked"; do
    rows="$(gh search issues \
      --owner "${GITHUB_OWNER}" \
      --label "${label}" \
      --state open \
      --json number,title,repository,updatedAt \
      2>/dev/null || echo "[]")"
    count="$(printf '%s' "${rows}" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))')"
    if [[ "${count}" -eq 0 ]]; then
      log "slow:   ${label}: none"
      continue
    fi
    while IFS= read -r line; do
      [[ -n "${line}" ]] && log "${line}"
    done < <(printf '%s' "${rows}" | python3 -c "${PY_REPORT_STALE}")
  done
}

# ---------------------------------------------------------------------------
# FAST PATH: single cross-repo search for queued issues; run one if found
# ---------------------------------------------------------------------------
fast_path() {
  if pgrep -f "codex exec" >/dev/null 2>&1; then
    log "fast: codex exec already running — skipping this wake"
    return 0
  fi

  # ONE cross-repo API call. Zero LLM tokens regardless of result.
  local rows
  rows="$(gh search issues \
    --owner "${GITHUB_OWNER}" \
    --label "agent/codex/queued" \
    --state open \
    --json number,title,repository,createdAt,labels \
    2>/dev/null || echo "[]")"

  local candidates
  candidates="$(printf '%s' "${rows}" | python3 -c "${PY_PICK_QUEUED}")"

  if [[ -z "${candidates}" ]]; then
    log "fast: queue empty (no config-repo queued issues) — no Codex invocation"
    return 0
  fi

  # Iterate candidates in priority order; pick the first one whose repo has a
  # local checkout and a clean working tree. Skip-and-continue on no-checkout
  # or dirty-tree so those repos never permanently starve the queue.
  local PRANK CREATED REPO ISSUE_NUM SHORT REPO_PATH
  local skipped_no_checkout=() skipped_dirty=() found=0
  while IFS=$'\t' read -r PRANK CREATED REPO ISSUE_NUM; do
    SHORT="$(repo_short_name "${REPO}")"
    REPO_PATH="${PROJECTS_ROOT}/${SHORT}"

    if [[ ! -d "${REPO_PATH}/.git" ]]; then
      skipped_no_checkout+=("${REPO}#${ISSUE_NUM}")
      log "fast: ${REPO}#${ISSUE_NUM} — no local checkout at ${REPO_PATH}, skipping (continuing to next)"
      continue
    fi

    if ! git -C "${REPO_PATH}" diff --quiet HEAD 2>/dev/null; then
      skipped_dirty+=("${REPO}#${ISSUE_NUM}")
      log "fast: ${REPO}#${ISSUE_NUM} — working tree dirty at ${REPO_PATH}, skipping (continuing to next)"
      continue
    fi

    found=1
    break
  done <<< "${candidates}"

  if [[ ${#skipped_no_checkout[@]} -gt 0 ]]; then
    log "fast: skipped (no checkout): ${skipped_no_checkout[*]}"
  fi
  if [[ ${#skipped_dirty[@]} -gt 0 ]]; then
    log "fast: skipped (dirty tree): ${skipped_dirty[*]}"
  fi

  if [[ "${found}" -eq 0 ]]; then
    log "fast: no eligible queued item (all skipped) — no Codex invocation"
    return 0
  fi

  log "fast: selected ${REPO}#${ISSUE_NUM} (priority ${PRANK}, created ${CREATED})"

  # TOCTOU re-check: fetch labels fresh right before claiming, in case a manual
  # session claimed this issue in the window between our search and our claim.
  local CURRENT_LABELS
  CURRENT_LABELS="$(gh issue view "${ISSUE_NUM}" --repo "${REPO}" --json labels --jq '[.labels[].name] | join(",")' 2>/dev/null || echo "")"
  if [[ "${CURRENT_LABELS}" == *"agent/codex/in-progress"* ]] || [[ "${CURRENT_LABELS}" != *"agent/codex/queued"* ]]; then
    log "fast: ${REPO}#${ISSUE_NUM} already claimed by another worker — skipping"
    return 0
  fi

  # Claim: queued -> in-progress BEFORE running (mutex)
  log "fast: claiming ${REPO}#${ISSUE_NUM}: queued -> in-progress"
  gh issue edit "${ISSUE_NUM}" --repo "${REPO}" \
    --add-label "agent/codex/in-progress" \
    --remove-label "agent/codex/queued" \
    2>>"${LOG_FILE}" || {
    log "fast: WARNING label transition failed — aborting to avoid double-run"
    return 1
  }

  # Isolated worktree off origin/main
  local WT_DIR BRANCH_NAME
  WT_DIR="${WORKTREES_ROOT}/${SHORT}-issue-${ISSUE_NUM}-$$"
  BRANCH_NAME="agent/codex/issue-${ISSUE_NUM}-poll"
  log "fast: creating worktree ${WT_DIR} (branch ${BRANCH_NAME} off origin/main)"
  git -C "${REPO_PATH}" fetch origin main >>"${LOG_FILE}" 2>&1 || log "fast: WARNING fetch failed, using local HEAD"
  git -C "${REPO_PATH}" worktree add "${WT_DIR}" -b "${BRANCH_NAME}" origin/main >>"${LOG_FILE}" 2>&1

  # Compact context seed via codex-handoff
  local HANDOFF_CONTEXT=""
  if [[ -x "${HANDOFF_BIN}" ]]; then
    HANDOFF_CONTEXT="$(${HANDOFF_BIN} --repo "${WT_DIR}" --root "${PROJECTS_ROOT}" --json 2>/dev/null || true)"
  fi

  local ISSUE_BODY
  ISSUE_BODY="$(gh issue view "${ISSUE_NUM}" --repo "${REPO}" --json body --jq '.body' 2>/dev/null || echo "")"

  local PROMPT
  PROMPT="You are Codex. Pick up this GitHub issue and execute it end-to-end.

ISSUE: ${REPO}#${ISSUE_NUM}
WORKING DIR: ${WT_DIR}

ISSUE BODY:
${ISSUE_BODY}

CONTEXT (codex-handoff):
${HANDOFF_CONTEXT}

Follow the 8-step loop in AGENTS.md (petersimmons1972/claude-codex). Post a CODEX REPORT block when done.
Worktree: ${WT_DIR} (branch: ${BRANCH_NAME} off origin/main). Do NOT touch the shared checkout at ${REPO_PATH}."

  local LOG_RUN
  LOG_RUN="${STATE_DIR}/${SHORT}-${ISSUE_NUM}-$(date -u +%Y%m%dT%H%M%S).log"
  log "fast: launching codex exec --ephemeral --cd ${WT_DIR} -> ${LOG_RUN}"
  codex exec --ephemeral --cd "${WT_DIR}" "${PROMPT}" > "${LOG_RUN}" 2>&1 &
  local CODEX_PID=$!
  log "fast: codex exec pid=${CODEX_PID} log=${LOG_RUN}"
  log "fast: done — Codex running in background, worktree owned by Codex"
}

# ---------------------------------------------------------------------------
# Dispatch
# ---------------------------------------------------------------------------
main() {
  if [[ -f "${KILL_SWITCH}" ]]; then
    log "kill switch active (${KILL_SWITCH}): exiting with no action"
    exit 0
  fi

  exec 9>"${LOCK_FILE}"
  if ! flock -n 9; then
    log "another codex-poll is running (lock held): exiting"
    exit 0
  fi

  if [[ ! -r "${TARGET_REPOS_FILE}" ]]; then
    log "ERROR: target-repos file not readable: ${TARGET_REPOS_FILE}"
    exit 1
  fi

  # Manual single-path mode for testing
  case "${1:-}" in
    fast)  fast_path; exit 0 ;;
    slow)  slow_path; exit 0 ;;
    daily) daily_path; exit 0 ;;
  esac

  # Tick counter drives tiered cadence from a single 5-min timer.
  local tick=0
  [[ -f "${TICK_FILE}" ]] && tick="$(cat "${TICK_FILE}" 2>/dev/null || echo 0)"
  tick=$(( (tick + 1) % 1000000 ))
  printf '%s\n' "${tick}" > "${TICK_FILE}"

  daily_path

  if (( tick % SLOW_EVERY == 0 )); then
    slow_path
  fi

  fast_path
}

main "$@"
