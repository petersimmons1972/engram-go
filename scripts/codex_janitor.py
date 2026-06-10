#!/usr/bin/env python3
"""Protocol 17 janitor implementation."""

import argparse
import json
import os
import shutil
import subprocess
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional, Sequence, Set, Tuple


CommandRunner = Callable[[Sequence[str]], str]


DONE_LABEL = "agent/codex/done"
CODEx_LABEL = "agent/codex"
PENDING_LABEL = "janitor/pending-close"
RUN_LABEL = "janitor/run"

DEFAULT_TARGET_REPOS_FILE = Path("~/.cache/claude-codex/config/target-repos.txt").expanduser()
DEFAULT_STATE_DIR = Path("~/.local/state/codex-poll/janitor").expanduser()
DEFAULT_TRACKING_ISSUE_FILE = Path("~/.local/state/codex-poll/janitor-tracking-issue.txt").expanduser()


@dataclass(frozen=True)
class JanitorIssue:
    repo: str
    number: int
    title: str
    url: str
    updated_at: Optional[datetime]
    labels: Set[str]


@dataclass(frozen=True)
class JanitorRun:
    auto_closed: List[str]
    flagged: List[str]
    branches_pruned: List[str]
    worktrees_pruned: List[str]
    skipped: List[str]


@dataclass(frozen=True)
class JanitorConfig:
    target_repos_file: Path
    stale_days: int
    pending_days: int
    state_dir: Path
    tracking_issue_file: Path
    tracking_issue_repo: str
    projects_root: Path
    worktrees_root: Path
    runner: CommandRunner


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


def load_target_repos(path: Path) -> List[str]:
    repos: List[str] = []
    for raw in path.read_text(encoding="utf-8").splitlines():
        value = raw.strip()
        if not value or value.startswith("#"):
            continue
        repos.append(value)
    return repos


def run_command(command: Sequence[str], *, runner: CommandRunner) -> str:
    return runner(command)


def fetch_json(command: Sequence[str], *, runner: CommandRunner) -> Any:
    raw = run_command(command, runner=runner).strip()
    if not raw:
        return []
    try:
        return json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValueError(f"failed to parse JSON for command: {command}") from exc


def parse_labels(raw_labels: Any) -> Set[str]:
    labels: Set[str] = set()
    if not isinstance(raw_labels, list):
        return labels
    for entry in raw_labels:
        if not isinstance(entry, dict):
            continue
        name = entry.get("name")
        if isinstance(name, str):
            labels.add(name)
    return labels


def list_issues(
    repo: str,
    *,
    runner: CommandRunner,
    label: Optional[str] = None,
) -> List[JanitorIssue]:
    command = [
        "gh",
        "issue",
        "list",
        "--repo",
        repo,
        "--state",
        "open",
        "--json",
        "number,title,url,labels,updatedAt",
        "--limit",
        "200",
    ]
    if label is not None:
        command.extend(["--label", label])
    payload = fetch_json(command, runner=runner)
    if not isinstance(payload, list):
        return []
    out: List[JanitorIssue] = []
    for raw_issue in payload:
        if not isinstance(raw_issue, dict):
            continue
        out.append(
            JanitorIssue(
                repo=repo,
                number=int(raw_issue["number"]),
                title=str(raw_issue.get("title", "")),
                url=str(raw_issue.get("url", "")),
                updated_at=parse_iso_datetime(raw_issue.get("updatedAt")),
                labels=parse_labels(raw_issue.get("labels")),
            )
        )
    return out


def list_run_signals(repos: list[str], *, runner: CommandRunner) -> Set[str]:
    flagged: Set[str] = set()
    for repo in repos:
        if list_issues(repo, runner=runner, label=RUN_LABEL):
            flagged.add(repo)
    return flagged


def issue_key(repo: str, number: int) -> str:
    return f"{repo.replace('/', '-') }#{number}"


def state_path(state_dir: Path, repo: str, number: int) -> Path:
    return state_dir / issue_key(repo, number)


def read_pending_flag(path: Path) -> Optional[datetime]:
    if not path.exists():
        return None
    raw = path.read_text(encoding="utf-8").strip()
    return parse_iso_datetime(raw)


def write_pending_flag(path: Path, *, at: datetime) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(at.isoformat(), encoding="utf-8")


def clear_pending_flag(path: Path) -> None:
    path.unlink(missing_ok=True)


def list_merged_pull_refs(repo: str, *, runner: CommandRunner) -> Set[str]:
    payload = fetch_json(
        ["gh", "api", f"repos/{repo}/pulls", "-f", "state=closed", "-f", "per_page=100", "--paginate"],
        runner=runner,
    )
    merged_refs: Set[str] = set()
    if not isinstance(payload, list):
        return merged_refs
    for raw_pr in payload:
        if not isinstance(raw_pr, dict):
            continue
        if not parse_iso_datetime(raw_pr.get("merged_at")):
            continue
        ref = raw_pr.get("head", {}).get("ref")
        if isinstance(ref, str):
            merged_refs.add(ref)
    return merged_refs


def list_branch_refs(repo: str, *, prefix: str, runner: CommandRunner) -> Set[str]:
    payload = fetch_json(
        ["gh", "api", f"repos/{repo}/git/matching-refs/heads/{prefix}"],
        runner=runner,
    )
    refs: Set[str] = set()
    if not isinstance(payload, list):
        return refs
    for entry in payload:
        if not isinstance(entry, dict):
            continue
        ref = entry.get("ref")
        if not isinstance(ref, str):
            continue
        if not ref.startswith("refs/heads/"):
            continue
        branch = ref.removeprefix("refs/heads/")
        if branch.startswith(prefix):
            refs.add(branch)
    return refs


def is_pull_merged(
    repo: str,
    pr_number: int,
    *,
    runner: CommandRunner,
    pull_cache: Dict[Tuple[str, int], bool],
) -> bool:
    cache_key = (repo, pr_number)
    if cache_key in pull_cache:
        return pull_cache[cache_key]
    payload = fetch_json(["gh", "api", f"repos/{repo}/pulls/{pr_number}"], runner=runner)
    merged = isinstance(payload, dict) and parse_iso_datetime(payload.get("merged_at")) is not None
    pull_cache[cache_key] = merged
    return merged


def issue_has_linked_merged_pr(
    issue: JanitorIssue,
    *,
    runner: CommandRunner,
    pull_cache: Dict[Tuple[str, int], bool],
) -> Optional[str]:
    payload = fetch_json(
        ["gh", "api", f"repos/{issue.repo}/issues/{issue.number}/timeline", "--paginate"],
        runner=runner,
    )
    if not isinstance(payload, list):
        return None
    for raw_event in payload:
        if not isinstance(raw_event, dict):
            continue
        if raw_event.get("event") != "cross-referenced":
            continue
        source_issue = raw_event.get("source", {}).get("issue")
        if not isinstance(source_issue, dict):
            continue
        pull_request = source_issue.get("pull_request")
        if not isinstance(pull_request, dict):
            continue
        pr_number = source_issue.get("number")
        if not isinstance(pr_number, int):
            continue
        if is_pull_merged(issue.repo, pr_number, runner=runner, pull_cache=pull_cache):
            pr_url = source_issue.get("html_url")
            return pr_url if isinstance(pr_url, str) else f"{issue.repo}#{pr_number}"
    return None


def comment_issue(issue: JanitorIssue, *, body: str, runner: CommandRunner) -> None:
    run_command(
        [
            "gh",
            "issue",
            "comment",
            str(issue.number),
            "--repo",
            issue.repo,
            "--body",
            body,
        ],
        runner=runner,
    )


def close_issue(issue: JanitorIssue, *, comment: str, runner: CommandRunner) -> None:
    run_command(
        [
            "gh",
            "issue",
            "close",
            str(issue.number),
            "--repo",
            issue.repo,
            "--comment",
            comment,
        ],
        runner=runner,
    )


def add_label(issue: JanitorIssue, *, label: str, runner: CommandRunner) -> None:
    run_command(
        [
            "gh",
            "issue",
            "edit",
            str(issue.number),
            "--repo",
            issue.repo,
            "--add-label",
            label,
        ],
        runner=runner,
    )


def remove_label(issue: JanitorIssue, *, label: str, runner: CommandRunner) -> None:
    run_command(
        [
            "gh",
            "issue",
            "edit",
            str(issue.number),
            "--repo",
            issue.repo,
            "--remove-label",
            label,
        ],
        runner=runner,
    )


def process_issue(
    issue: JanitorIssue,
    *,
    stale_days: int,
    pending_days: int,
    now: datetime,
    config: JanitorConfig,
    pull_cache: Dict[Tuple[str, int], bool],
) -> Tuple[Optional[str], Optional[str], Optional[str]]:
    pending_path = state_path(config.state_dir, issue.repo, issue.number)
    pending_flag_time = read_pending_flag(pending_path)
    is_done = DONE_LABEL in issue.labels
    merged_pr_url = None
    if is_done:
        merged_pr_url = issue_has_linked_merged_pr(
            issue,
            runner=config.runner,
            pull_cache=pull_cache,
        )

    if is_done and merged_pr_url:
        close_issue(
            issue,
            comment=f"Auto-closed by Protocol 17 janitor. Linked merged PR: {merged_pr_url}",
            runner=config.runner,
        )
        clear_pending_flag(pending_path)
        if PENDING_LABEL in issue.labels:
            remove_label(issue, label=PENDING_LABEL, runner=config.runner)
        return (f"{issue.repo}#{issue.number}", None, None)

    if issue.updated_at is None:
        return (None, None, f"skipped:{issue.repo}#{issue.number}: missing updatedAt")

    age = now - issue.updated_at
    if age < timedelta(days=stale_days):
        return (None, None, f"skipped:{issue.repo}#{issue.number}: not stale ({age.days} days)")

    if PENDING_LABEL not in issue.labels:
        comment_issue(
            issue,
            body=(
                f"⚠️ Stale/blocked check: this issue is idle {age.days} days with labels "
                f"{', '.join(sorted(issue.labels)) if issue.labels else 'none'}. "
                "Janitor will close after 7 days of inactivity; update this issue now "
                "to pause."
            ),
            runner=config.runner,
        )
        add_label(issue, label=PENDING_LABEL, runner=config.runner)
        write_pending_flag(pending_path, at=now)
        return (None, f"{issue.repo}#{issue.number}", None)

    if pending_flag_time is None:
        write_pending_flag(pending_path, at=now)
        return (None, f"{issue.repo}#{issue.number}", None)

    if now - pending_flag_time >= timedelta(days=pending_days):
        close_issue(
            issue,
            comment="Auto-closed by Protocol 17 janitor after 7 days with no activity.",
            runner=config.runner,
        )
        clear_pending_flag(pending_path)
        remove_label(issue, label=PENDING_LABEL, runner=config.runner)
        return (f"{issue.repo}#{issue.number}", None, None)

    return (None, None, f"skipped:{issue.repo}#{issue.number}: waiting for timer")


def prune_branch(repo: str, branch: str, *, repo_dir: Path, runner: CommandRunner) -> bool:
    if not repo_dir.exists():
        return False
    run_command(
        ["git", "-C", str(repo_dir), "push", "origin", "--delete", branch],
        runner=runner,
    )
    return True


def prune_worktree(path: Path) -> None:
    shutil.rmtree(path)


def detect_worktree_branch(path: Path, *, runner: CommandRunner) -> Optional[str]:
    return run_command(
        ["git", "-C", str(path), "symbolic-ref", "--short", "HEAD"],
        runner=runner,
    ).strip() or None


def summarize_run(
    auto_closed: list[str],
    flagged: list[str],
    branches_pruned: list[str],
    worktrees_pruned: list[str],
    skipped: list[str],
) -> str:
    lines = [
        "### Janitor run summary",
        f"- Auto-closed: {len(auto_closed)}",
        f"- Flagged: {len(flagged)}",
        f"- Branches pruned: {len(branches_pruned)}",
        f"- Worktrees pruned: {len(worktrees_pruned)}",
        f"- Items skipped: {len(skipped)}",
        "",
        "#### Auto-closed issues",
    ]
    lines.extend(f"- {item}" for item in auto_closed) if auto_closed else lines.append("- _None_")
    lines.extend(["", "#### Flagged issues"])
    lines.extend(f"- {item}" for item in flagged) if flagged else lines.append("- _None_")
    lines.extend(["", "#### Branches pruned"])
    lines.extend(f"- {item}" for item in branches_pruned) if branches_pruned else lines.append("- _None_")
    lines.extend(["", "#### Worktrees pruned"])
    lines.extend(f"- {item}" for item in worktrees_pruned) if worktrees_pruned else lines.append("- _None_")
    lines.extend(["", "#### Items skipped with reason"])
    lines.extend(f"- {item}" for item in skipped) if skipped else lines.append("- _None_")
    return "\n".join(lines)


def read_tracking_issue(config: JanitorConfig) -> Optional[Tuple[str, int]]:
    if not config.tracking_issue_file.exists():
        return None
    raw = config.tracking_issue_file.read_text(encoding="utf-8").strip()
    if not raw:
        return None
    try:
        return (config.tracking_issue_repo, int(raw))
    except ValueError:
        return None


def post_summary(tracking_issue: Tuple[str, int], summary: str, *, config: JanitorConfig) -> None:
    repo, number = tracking_issue
    run_command(
        [
            "gh",
            "issue",
            "comment",
            str(number),
            "--repo",
            repo,
            "--body",
            summary,
        ],
        runner=config.runner,
    )


def run_janitor(config: JanitorConfig, *, now: Optional[datetime] = None) -> JanitorRun:
    if now is None:
        now = now_utc()
    repos = load_target_repos(config.target_repos_file)
    run_signals = list_run_signals(repos, runner=config.runner)
    _ = run_signals

    auto_closed: List[str] = []
    flagged: List[str] = []
    branches_pruned: List[str] = []
    worktrees_pruned: List[str] = []
    skipped: List[str] = []

    merged_refs_by_repo: Dict[str, Set[str]] = {}
    pull_cache: Dict[Tuple[str, int], bool] = {}

    for repo in repos:
        merged_refs = list_merged_pull_refs(repo, runner=config.runner)
        merged_refs_by_repo[repo] = merged_refs
        for issue in list_issues(repo, runner=config.runner, label=CODEx_LABEL):
            auto_closed_item, flagged_item, skipped_item = process_issue(
                issue,
                stale_days=config.stale_days,
                pending_days=config.pending_days,
                now=now,
                config=config,
                pull_cache=pull_cache,
            )
            if auto_closed_item is not None:
                auto_closed.append(auto_closed_item)
            if flagged_item is not None:
                flagged.append(flagged_item)
            if skipped_item is not None:
                skipped.append(skipped_item)

        repo_dir = config.projects_root / repo.split("/")[-1]
        for branch in list_branch_refs(repo, prefix="agent/codex/", runner=config.runner):
            if branch in merged_refs:
                if prune_branch(repo, branch, repo_dir=repo_dir, runner=config.runner):
                    branches_pruned.append(f"{repo}:{branch}")

    all_merged_refs: Set[str] = set()
    for refs in merged_refs_by_repo.values():
        all_merged_refs.update(refs)

    if config.worktrees_root.exists():
        for path in config.worktrees_root.iterdir():
            if not path.is_dir():
                continue
            branch = detect_worktree_branch(path, runner=config.runner)
            if not branch or not branch.startswith("agent/hermes/"):
                continue
            if branch in all_merged_refs:
                prune_worktree(path)
                worktrees_pruned.append(str(path))

    tracking_issue = read_tracking_issue(config)
    summary = summarize_run(auto_closed, flagged, branches_pruned, worktrees_pruned, skipped)
    if tracking_issue is not None:
        post_summary(tracking_issue, summary, config=config)
    return JanitorRun(
        auto_closed=auto_closed,
        flagged=flagged,
        branches_pruned=branches_pruned,
        worktrees_pruned=worktrees_pruned,
        skipped=skipped,
    )


def default_config() -> JanitorConfig:
    stale_days_raw = os.getenv("JANITOR_STALE_DAYS", "14")
    pending_days_raw = os.getenv("JANITOR_PENDING_DAYS", "7")
    try:
        stale_days = int(stale_days_raw)
    except ValueError:
        stale_days = 14
    try:
        pending_days = int(pending_days_raw)
    except ValueError:
        pending_days = 7

    projects_root = Path(os.getenv("PROJECTS_ROOT", str(Path.home() / "projects"))).expanduser()
    worktrees_root = Path(os.getenv("WORKTREES_ROOT", str(Path.home() / "projects/.codex-poll-worktrees"))).expanduser()

    def _runner(cmd: Sequence[str]) -> str:
        completed = subprocess.run(list(cmd), capture_output=True, text=True, check=True)
        return completed.stdout.strip()

    return JanitorConfig(
        target_repos_file=DEFAULT_TARGET_REPOS_FILE,
        stale_days=stale_days,
        pending_days=pending_days,
        state_dir=DEFAULT_STATE_DIR,
        tracking_issue_file=DEFAULT_TRACKING_ISSUE_FILE,
        tracking_issue_repo=os.getenv("JANITOR_TRACKING_REPO", "petersimmons1972/claude-codex"),
        projects_root=projects_root,
        worktrees_root=worktrees_root,
        runner=_runner,
    )


def build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run Protocol 17 janitor")
    parser.add_argument("--once", action="store_true", help="run one janitor cycle")
    return parser


def main() -> int:
    build_arg_parser().parse_args()
    run_janitor(default_config())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
