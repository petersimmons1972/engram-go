#!/usr/bin/env python3
"""Protocol 16 founder digest generator."""

from __future__ import annotations

import argparse
import json
import logging
import os
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Callable, Dict, Iterable, List, Optional, Sequence, Tuple


CommandRunner = Callable[[Sequence[str]], str]


DONE_LABEL = "agent/codex/done"
BLOCKED_LABELS = ("agent/codex/blocked", "agent/codex/in-progress")
NEEDS_YOU_LABELS = ("decision/needs-founder", "agent/codex/needs-claude")
DEFAULT_TARGET_REPOS_FILE = Path("~/.cache/claude-codex/config/target-repos.txt").expanduser()
DEFAULT_TRACKING_ISSUE_FILE = Path("~/.local/state/codex-poll/digest-tracking-issue.txt").expanduser()
DEFAULT_TRACKING_REPO = "petersimmons1972/claude-codex"
TRACKING_ISSUE_TITLE = "Founder digest tracking"
TRACKING_ISSUE_BODY = "Automated founder digest comments land here."


@dataclass(frozen=True)
class DigestItem:
    repo: str
    number: int
    title: str
    url: str
    label: str
    updated_at: Optional[datetime]
    pr_url: Optional[str] = None
    pr_merged_at: Optional[datetime] = None
    closed_at: Optional[datetime] = None


def now_utc() -> datetime:
    return datetime.now(timezone.utc)


def parse_iso_datetime(value: Optional[str]) -> Optional[datetime]:
    if not value:
        return None
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(value)
    except (TypeError, ValueError):
        return None


def run_command(command: Sequence[str], *, runner: Optional[CommandRunner] = None) -> str:
    if runner is not None:
        return runner(command)
    completed = subprocess.run(
        list(command),
        check=True,
        text=True,
        capture_output=True,
    )
    return completed.stdout


def fetch_json(command: Sequence[str], *, runner: Optional[CommandRunner] = None) -> Any:
    raw = run_command(command, runner=runner).strip()
    if not raw:
        return []
    return json.loads(raw)


def load_target_repos(path: Path) -> List[str]:
    repos: List[str] = []
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        repos.append(line)
    return repos


def parse_labels(raw_labels: Any) -> List[str]:
    labels: List[str] = []
    if not isinstance(raw_labels, list):
        return labels
    for raw_label in raw_labels:
        if not isinstance(raw_label, dict):
            continue
        name = raw_label.get("name")
        if isinstance(name, str):
            labels.append(name)
    return labels


def parse_issue(repo: str, raw_issue: dict[str, Any], *, preferred_label: str) -> DigestItem:
    return DigestItem(
        repo=repo,
        number=int(raw_issue["number"]),
        title=str(raw_issue.get("title", "")),
        url=str(raw_issue.get("url", "")),
        label=preferred_label,
        updated_at=parse_iso_datetime(raw_issue.get("updatedAt")),
        closed_at=parse_iso_datetime(raw_issue.get("closedAt")),
    )


def list_issues(repo: str, *, state: str, label: str, runner: Optional[CommandRunner] = None) -> List[DigestItem]:
    payload = fetch_json(
        [
            "gh",
            "issue",
            "list",
            "--repo",
            repo,
            "--state",
            state,
            "--label",
            label,
            "--json",
            "number,title,url,labels,updatedAt,closedAt",
            "--limit",
            "200",
        ],
        runner=runner,
    )
    if not isinstance(payload, list):
        return []
    return [parse_issue(repo, raw_issue, preferred_label=label) for raw_issue in payload if isinstance(raw_issue, dict)]


def dedupe_items(items: Iterable[DigestItem]) -> List[DigestItem]:
    unique: Dict[Tuple[str, int], DigestItem] = {}
    for item in items:
        unique[(item.repo, item.number)] = item
    return list(unique.values())


def issue_age_days(updated_at: Optional[datetime], *, now: datetime) -> str:
    # GitHub's issue list payload does not expose label-applied timestamps, so
    # Protocol 16 uses updatedAt as the required age proxy for these counters.
    if updated_at is None:
        return "unknown days"
    delta = now - updated_at
    days = max(0, delta.days)
    return f"{days} days"


def render_shipped_item(item: DigestItem) -> str:
    merged_date = item.pr_merged_at.date().isoformat() if item.pr_merged_at else "unknown"
    pr_url = item.pr_url or "-"
    return f"- [{item.repo}#{item.number}]({item.url}) {item.title} | PR: {pr_url} | merged: {merged_date}"


def render_stateful_item(item: DigestItem, *, now: datetime) -> str:
    age = issue_age_days(item.updated_at, now=now)
    return f"- [{item.repo}#{item.number}]({item.url}) {item.title} | label: {item.label} | age: {age}"


def render_section(title: str, items: List[DigestItem], *, now: datetime, kind: str) -> List[str]:
    lines = [f"## {title}"]
    if not items:
        lines.append("_None this period._")
        return lines
    renderer = render_shipped_item if kind == "shipped" else lambda item: render_stateful_item(item, now=now)
    lines.extend(renderer(item) for item in items)
    return lines


def render_digest(
    *,
    generated_at: datetime,
    shipped: List[DigestItem],
    blocked: List[DigestItem],
    needs_you: List[DigestItem],
    now: datetime,
) -> str:
    lines = [f"# Founder digest ({generated_at.date().isoformat()})"]
    lines.extend(render_section("Shipped", shipped, now=now, kind="shipped"))
    lines.extend(render_section("Blocked / in-flight", blocked, now=now, kind="blocked"))
    lines.extend(render_section("Needs you", needs_you, now=now, kind="needs"))
    if len(lines) > 30:
        logging.warning("digest exceeds target length: %s lines", len(lines))
    return "\n".join(lines)


def select_shipped_issues(items: Iterable[DigestItem]) -> List[DigestItem]:
    selected = [item for item in items if item.pr_url and item.pr_merged_at]
    selected.sort(key=lambda item: item.pr_merged_at or datetime.min.replace(tzinfo=timezone.utc), reverse=True)
    return selected


def read_tracking_issue_number(tracking_issue_file: Path) -> Optional[int]:
    if not tracking_issue_file.exists():
        return None
    raw = tracking_issue_file.read_text(encoding="utf-8").strip()
    if not raw:
        return None
    try:
        return int(raw)
    except ValueError:
        return None


def ensure_tracking_issue(
    *,
    repo: str,
    tracking_issue_file: Path,
    runner: Optional[CommandRunner] = None,
) -> int:
    existing = read_tracking_issue_number(tracking_issue_file)
    if existing is not None:
        return existing
    payload = fetch_json(
        [
            "gh",
            "api",
            f"repos/{repo}/issues",
            "-f",
            f"title={TRACKING_ISSUE_TITLE}",
            "-f",
            f"body={TRACKING_ISSUE_BODY}",
            "-f",
            "labels[]=digest",
        ],
        runner=runner,
    )
    if not isinstance(payload, dict) or not isinstance(payload.get("number"), int):
        raise ValueError("failed to create tracking issue")
    tracking_issue_file.parent.mkdir(parents=True, exist_ok=True)
    tracking_issue_file.write_text(f"{payload['number']}\n", encoding="utf-8")
    return int(payload["number"])


def post_digest(
    *,
    repo: str,
    tracking_issue_file: Path,
    digest_body: str,
    runner: Optional[CommandRunner] = None,
) -> None:
    issue_number = read_tracking_issue_number(tracking_issue_file)
    if issue_number is None:
        raise ValueError("tracking issue number is not recorded")
    run_command(
        [
            "gh",
            "issue",
            "comment",
            str(issue_number),
            "--repo",
            repo,
            "--body",
            digest_body,
        ],
        runner=runner,
    )


def pull_repo_slug(source_issue: dict[str, Any], fallback_repo: str) -> str:
    repository_url = source_issue.get("repository_url")
    if isinstance(repository_url, str) and "/repos/" in repository_url:
        tail = repository_url.split("/repos/", 1)[1]
        parts = tail.split("/")
        if len(parts) >= 2:
            return f"{parts[0]}/{parts[1]}"
    return fallback_repo


def fetch_linked_merged_pr(
    item: DigestItem,
    *,
    runner: Optional[CommandRunner] = None,
    pull_cache: Optional[Dict[Tuple[str, int], dict[str, Any]]] = None,
) -> Optional[DigestItem]:
    if pull_cache is None:
        pull_cache = {}
    payload = fetch_json(
        ["gh", "api", f"repos/{item.repo}/issues/{item.number}/timeline", "--paginate"],
        runner=runner,
    )
    if not isinstance(payload, list):
        return None
    for raw_event in payload:
        if not isinstance(raw_event, dict) or raw_event.get("event") != "cross-referenced":
            continue
        source_issue = raw_event.get("source", {}).get("issue")
        if not isinstance(source_issue, dict) or "pull_request" not in source_issue:
            continue
        pr_number = source_issue.get("number")
        if not isinstance(pr_number, int):
            continue
        pr_repo = pull_repo_slug(source_issue, item.repo)
        cache_key = (pr_repo, pr_number)
        pr_payload = pull_cache.get(cache_key)
        if pr_payload is None:
            fetched = fetch_json(["gh", "api", f"repos/{pr_repo}/pulls/{pr_number}"], runner=runner)
            if not isinstance(fetched, dict):
                continue
            pr_payload = fetched
            pull_cache[cache_key] = pr_payload
        merged_at = parse_iso_datetime(pr_payload.get("merged_at"))
        if merged_at is None:
            continue
        html_url = pr_payload.get("html_url")
        if not isinstance(html_url, str):
            continue
        return DigestItem(
            repo=item.repo,
            number=item.number,
            title=item.title,
            url=item.url,
            label=item.label,
            updated_at=item.updated_at,
            pr_url=html_url,
            pr_merged_at=merged_at,
            closed_at=item.closed_at,
        )
    return None


def gather_shipped(
    repos: Sequence[str],
    *,
    since: datetime,
    runner: Optional[CommandRunner] = None,
) -> List[DigestItem]:
    shipped: List[DigestItem] = []
    pull_cache: Dict[Tuple[str, int], dict[str, Any]] = {}
    for repo in repos:
        for item in list_issues(repo, state="closed", label=DONE_LABEL, runner=runner):
            if item.closed_at is None or item.closed_at < since:
                continue
            linked = fetch_linked_merged_pr(item, runner=runner, pull_cache=pull_cache)
            if linked is not None:
                shipped.append(linked)
    return select_shipped_issues(shipped)


def gather_open_section(
    repos: Sequence[str],
    *,
    labels: Sequence[str],
    runner: Optional[CommandRunner] = None,
) -> List[DigestItem]:
    items: List[DigestItem] = []
    for repo in repos:
        for label in labels:
            items.extend(list_issues(repo, state="open", label=label, runner=runner))
    deduped = dedupe_items(items)
    deduped.sort(key=lambda item: (item.updated_at or datetime.min.replace(tzinfo=timezone.utc)), reverse=False)
    return deduped


def build_digest(
    *,
    target_repos_file: Path,
    now: datetime,
    since_days: int,
    runner: Optional[CommandRunner] = None,
) -> str:
    repos = load_target_repos(target_repos_file)
    since = now - timedelta(days=since_days)
    shipped = gather_shipped(repos, since=since, runner=runner)
    blocked = gather_open_section(repos, labels=BLOCKED_LABELS, runner=runner)
    needs_you = gather_open_section(repos, labels=NEEDS_YOU_LABELS, runner=runner)
    return render_digest(
        generated_at=now,
        shipped=shipped,
        blocked=blocked,
        needs_you=needs_you,
        now=now,
    )


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate and post the Codex founder digest.")
    parser.add_argument("--once", action="store_true", help="Run one digest cycle and exit.")
    parser.add_argument("--dry-run", action="store_true", help="Render the digest to stdout without posting.")
    parser.add_argument("--days", type=int, default=7, help="Lookback window for shipped issues.")
    parser.add_argument(
        "--target-repos-file",
        type=Path,
        default=Path(os.environ.get("CODEX_DIGEST_TARGET_REPOS_FILE", DEFAULT_TARGET_REPOS_FILE)).expanduser(),
        help="Path to the target repo list.",
    )
    parser.add_argument(
        "--tracking-issue-file",
        type=Path,
        default=Path(os.environ.get("CODEX_DIGEST_TRACKING_ISSUE_FILE", DEFAULT_TRACKING_ISSUE_FILE)).expanduser(),
        help="Path storing the tracking issue number.",
    )
    parser.add_argument(
        "--tracking-repo",
        default=os.environ.get("CODEX_DIGEST_TRACKING_REPO", DEFAULT_TRACKING_REPO),
        help="Repo that holds the digest tracking issue.",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
    args = parse_args(argv)
    current_time = now_utc()
    digest_body = build_digest(
        target_repos_file=args.target_repos_file,
        now=current_time,
        since_days=args.days,
    )
    if args.dry_run:
        print(digest_body)
        return 0
    ensure_tracking_issue(repo=args.tracking_repo, tracking_issue_file=args.tracking_issue_file)
    post_digest(
        repo=args.tracking_repo,
        tracking_issue_file=args.tracking_issue_file,
        digest_body=digest_body,
    )
    if args.once:
        return 0
    return 0


if __name__ == "__main__":
    sys.exit(main())
