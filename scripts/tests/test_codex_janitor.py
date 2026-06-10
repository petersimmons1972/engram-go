from __future__ import annotations

import importlib.util
import sys
import json
from pathlib import Path
from typing import Any, Callable, Dict, Optional, Sequence, Tuple


def _module():
    path = Path(__file__).resolve().parents[1] / "codex_janitor.py"
    spec = importlib.util.spec_from_file_location("codex_janitor", path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules["codex_janitor"] = module
    spec.loader.exec_module(module)  # type: ignore[arg-type]
    return module


def _build_issue(number: int, title: str, labels: list[str], updated_at: str, repo: str = "petersimmons1972/homelab-config") -> dict[str, Any]:
    return {
        "number": number,
        "title": title,
        "url": f"https://github.com/{repo}/issues/{number}",
        "labels": [{"name": label} for label in labels],
        "updatedAt": updated_at,
    }


class FakeRunner:
    def __init__(
        self,
        *,
        issues: Dict[Tuple[str, Optional[str]], list[dict[str, Any]]],
        timelines: Dict[Tuple[str, int], list[dict[str, Any]]],
        pulls: Dict[str, list[dict[str, Any]]],
        pull_by_number: Dict[Tuple[str, int], dict[str, Any]],
        branch_refs: Dict[Tuple[str, str], list[str]],
        worktree_branches: Dict[str, str],
    ) -> None:
        self.issues = issues
        self.timelines = timelines
        self.pulls = pulls
        self.pull_by_number = pull_by_number
        self.branch_refs = branch_refs
        self.worktree_branches = worktree_branches
        self.calls: list[list[str]] = []

    def __call__(self, command: Sequence[str]) -> str:
        cmd = list(command)
        self.calls.append(cmd)

        if cmd[:2] == ["gh", "issue"] and "list" in cmd:
            repo = cmd[cmd.index("--repo") + 1]
            label = None
            if "--label" in cmd:
                label = cmd[cmd.index("--label") + 1]
            return json.dumps(self.issues.get((repo, label), []))

        if cmd[:2] == ["gh", "issue"] and cmd[2] == "close":
            return ""
        if cmd[:2] == ["gh", "issue"] and cmd[2] == "comment":
            return ""
        if cmd[:2] == ["gh", "issue"] and cmd[2] == "edit":
            return ""
        if cmd[:2] == ["gh", "api"]:
            endpoint = cmd[2]
            if endpoint.endswith("/timeline"):
                endpoint_parts = endpoint.split("/")
                # repos/{owner}/{repo}/issues/{number}/timeline
                number = int(endpoint_parts[-2])
                repo = "/".join(endpoint_parts[1:3])
                return json.dumps(self.timelines.get((repo, int(number)), []))
            if "/pulls/" in endpoint:
                endpoint_parts = endpoint.split("/")
                # repos/{owner}/{repo}/pulls/{number}
                repo = "/".join(endpoint_parts[1:3])
                number = int(endpoint_parts[4])
                return json.dumps(self.pull_by_number[(repo, number)])
            if endpoint.endswith("/pulls"):
                # repos/{owner}/{repo}/pulls
                repo = "/".join(endpoint.split("/")[1:3])
                return json.dumps(self.pulls.get(repo, []))
            if "/git/matching-refs/heads/" in endpoint:
                # repos/{owner}/{repo}/git/matching-refs/heads/{prefix}
                repo = "/".join(endpoint.split("/")[1:3])
                prefix = endpoint.split("/matching-refs/heads/", 1)[1]
                values = self.branch_refs.get((repo, prefix), [])
                return json.dumps([{"ref": f"refs/heads/{branch}"} for branch in values])
        if cmd[:1] == ["git"]:
            # git branch checks are only symbolic-ref checks for fake worktrees.
            if "symbolic-ref" in cmd:
                path = cmd[2]
                return self.worktree_branches.get(path, "")
            return ""
        return ""

    def command_count(self, *expected_prefix: str) -> int:
        return sum(cmd[: len(expected_prefix)] == list(expected_prefix) for cmd in self.calls)


def _config(tmp_path: Path, module, run: FakeRunner):
    state_dir = tmp_path / "janitor-state"
    target = tmp_path / "target-repos.txt"
    target.write_text("petersimmons1972/homelab-config\npetersimmons1972/ollama\n", encoding="utf-8")
    return module.JanitorConfig(
        target_repos_file=target,
        stale_days=14,
        pending_days=7,
        state_dir=state_dir,
        tracking_issue_file=tmp_path / "janitor-tracking-issue.txt",
        tracking_issue_repo="petersimmons1972/claude-codex",
        projects_root=tmp_path / "projects",
        worktrees_root=tmp_path / "worktrees",
        runner=run,
    )


def test_auto_close_only_when_done_and_pr_merged(tmp_path: Path) -> None:
    module = _module()
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [
            _build_issue(75, "done and merged", ["agent/codex", "agent/codex/done"], "2026-05-01T00:00:00Z"),
        ],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    timelines = {("petersimmons1972/homelab-config", 75): [{"event": "cross-referenced", "source": {"issue": {"number": 8, "pull_request": {"url": "x"}, "html_url": "https://x"}}}]}
    pull_by_number = {("petersimmons1972/homelab-config", 8): {"merged_at": "2026-06-01T00:00:00Z", "html_url": "https://x"}}
    runner = FakeRunner(
        issues=issues,
        timelines=timelines,
        pulls={},
        pull_by_number=pull_by_number,
        branch_refs={},
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)
    result = module.run_janitor(cfg, now=module.datetime(2026, 6, 7, tzinfo=module.timezone.utc))

    assert result.auto_closed == ["petersimmons1972/homelab-config#75"]
    assert runner.command_count("gh", "issue", "close") == 1
    assert any(cmd[0:3] == ["gh", "issue", "close"] for cmd in runner.calls)


def test_flag_not_auto_close_when_no_merged_pr(tmp_path: Path) -> None:
    module = _module()
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [
            _build_issue(75, "no merged link", ["agent/codex", "agent/codex/done"], "2026-05-01T00:00:00Z"),
        ],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={("petersimmons1972/homelab-config", 75): []},
        pulls={},
        pull_by_number={},
        branch_refs={},
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)
    now = module.datetime(2026, 6, 7, tzinfo=module.timezone.utc)
    result = module.run_janitor(cfg, now=now)

    assert result.auto_closed == []
    assert result.flagged == ["petersimmons1972/homelab-config#75"]
    assert (cfg.state_dir / module.issue_key("petersimmons1972/homelab-config", 75)).exists()
    assert any(cmd[:3] == ["gh", "issue", "comment"] for cmd in runner.calls)
    assert not any(cmd[:3] == ["gh", "issue", "close"] for cmd in runner.calls)


def test_flag_then_timer_flow_closes_after_pending_days(tmp_path: Path) -> None:
    module = _module()
    issue = _build_issue(81, "stale issue", ["agent/codex"], "2026-05-01T00:00:00Z")
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [issue],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={("petersimmons1972/homelab-config", 81): []},
        pulls={},
        pull_by_number={},
        branch_refs={},
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)

    now1 = module.datetime(2026, 6, 7, tzinfo=module.timezone.utc)
    first = module.run_janitor(cfg, now=now1)
    assert first.flagged == ["petersimmons1972/homelab-config#81"]
    assert not any(cmd[:3] == ["gh", "issue", "close"] for cmd in runner.calls)

    issues[("petersimmons1972/homelab-config", module.CODEx_LABEL)] = [_build_issue(81, "stale issue", ["agent/codex", module.PENDING_LABEL], "2026-05-01T00:00:00Z")]
    now2 = module.datetime(2026, 6, 15, tzinfo=module.timezone.utc)
    second = module.run_janitor(cfg, now=now2)
    assert second.auto_closed == ["petersimmons1972/homelab-config#81"]
    assert any(cmd[:3] == ["gh", "issue", "close"] for cmd in runner.calls)


def test_no_double_flag_for_pending_issue(tmp_path: Path) -> None:
    module = _module()
    issue = _build_issue(82, "already pending", ["agent/codex", module.PENDING_LABEL], "2026-05-01T00:00:00Z")
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [issue],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={},
        pulls={},
        pull_by_number={},
        branch_refs={},
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)
    flag_file = cfg.state_dir / module.issue_key("petersimmons1972/homelab-config", 82)
    flag_file.parent.mkdir(parents=True, exist_ok=True)
    flag_file.write_text((module.datetime(2026, 6, 1, tzinfo=module.timezone.utc)).isoformat(), encoding="utf-8")

    now = module.datetime(2026, 6, 7, tzinfo=module.timezone.utc)
    result = module.run_janitor(cfg, now=now)

    assert result.auto_closed == []
    assert not any(cmd[:3] == ["gh", "issue", "comment"] for cmd in runner.calls)


def test_branch_prune_restricts_to_merged_agent_codox_refs(tmp_path: Path) -> None:
    module = _module()
    repo_dir = tmp_path / "projects" / "homelab-config"
    repo_dir.mkdir(parents=True, exist_ok=True)
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={},
        pulls={
            "petersimmons1972/homelab-config": [
                {"head": {"ref": "agent/codex/merged-branch"}, "merged_at": "2026-06-01T00:00:00Z"},
                {"head": {"ref": "feature/other"}, "merged_at": None},
            ],
            "petersimmons1972/ollama": [
                {"head": {"ref": "agent/codex/never-delete"}, "merged_at": "2026-06-01T00:00:00Z"},
            ],
        },
        pull_by_number={},
        branch_refs={
            ("petersimmons1972/homelab-config", "agent/codex/"): ["agent/codex/merged-branch", "feature/other"],
            ("petersimmons1972/ollama", "agent/codex/"): ["agent/codex/never-delete"],
        },
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)
    (tmp_path / "projects" / "ollama").mkdir(parents=True, exist_ok=True)
    result = module.run_janitor(cfg, now=module.datetime(2026, 6, 7, tzinfo=module.timezone.utc))

    assert result.branches_pruned == ["petersimmons1972/homelab-config:agent/codex/merged-branch", "petersimmons1972/ollama:agent/codex/never-delete"]
    assert not any("feature/other" in cmd for cmd in map(str, runner.calls))


def test_worktree_prune_only_agent_hermes_merged_prefix(tmp_path: Path) -> None:
    module = _module()
    wt_root = tmp_path / "worktrees"
    wt1 = wt_root / "homelab-config-issue-1"
    wt2 = wt_root / "other"
    wt1.mkdir(parents=True)
    wt2.mkdir(parents=True)
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={},
        pulls={"petersimmons1972/homelab-config": [{"head": {"ref": "agent/hermes/keep-worktree"}, "merged_at": "2026-06-01T00:00:00Z"}]},
        pull_by_number={},
        branch_refs={},
        worktree_branches={
            str(wt1): "agent/hermes/keep-worktree",
            str(wt2): "feature/other",
        },
    )
    cfg = _config(tmp_path, module, runner)
    result = module.run_janitor(cfg, now=module.datetime(2026, 6, 7, tzinfo=module.timezone.utc))

    assert not wt1.exists()
    assert wt2.exists()
    assert result.worktrees_pruned == [str(wt1)]


def test_run_summary_is_posted_and_lists_categories(tmp_path: Path) -> None:
    module = _module()
    tracking_repo = "petersimmons1972/claude-codex"
    (tmp_path / "janitor-tracking-issue.txt").write_text("42", encoding="utf-8")

    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [
            _build_issue(80, "done merged", ["agent/codex", "agent/codex/done"], "2026-05-01T00:00:00Z"),
            _build_issue(81, "stale open", ["agent/codex"], "2026-05-01T00:00:00Z"),
        ],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    timelines = {
        ("petersimmons1972/homelab-config", 80): [{"event": "cross-referenced", "source": {"issue": {"number": 21, "pull_request": {"url": "x"}, "html_url": "https://x"}}}],
    }
    runner = FakeRunner(
        issues=issues,
        timelines=timelines,
        pulls={},
        pull_by_number={("petersimmons1972/homelab-config", 21): {"merged_at": "2026-06-01T00:00:00Z", "html_url": "https://x"}},
        branch_refs={("petersimmons1972/homelab-config", "agent/codex/"): ["agent/codex/cleanup"]},
        worktree_branches={},
    )
    runner.pulls["petersimmons1972/homelab-config"] = [{"head": {"ref": "agent/codex/cleanup"}, "merged_at": "2026-06-01T00:00:00Z"}]
    cfg = _config(tmp_path, module, runner)

    repo_dir = tmp_path / "projects" / "homelab-config"
    repo_dir.mkdir(parents=True, exist_ok=True)
    result = module.run_janitor(cfg, now=module.datetime(2026, 6, 7, tzinfo=module.timezone.utc))

    comment_calls = [cmd for cmd in runner.calls if cmd[:3] == ["gh", "issue", "comment"]]
    assert comment_calls, "expected run summary comment"
    summary = next(call[-1] for call in comment_calls if len(call) > 1 and call[0] == "gh" and call[2] == "comment" and call[3] == "42")
    assert "### Janitor run summary" in summary
    assert "- Auto-closed: 1" in summary
    assert "- Flagged: 1" in summary
    assert "- Branches pruned: 1" in summary
    assert "- petersimmons1972/homelab-config#80" in summary
    assert "agent/codex/cleanup" in summary

    assert result.auto_closed == ["petersimmons1972/homelab-config#80"]
    assert result.flagged == ["petersimmons1972/homelab-config#81"]


def test_janitor_run_label_fires_all_repos(tmp_path: Path) -> None:
    module = _module()
    target_repos = "petersimmons1972/homelab-config\npetersimmons1972/ollama\n"
    target = tmp_path / "target-repos.txt"
    target.write_text(target_repos, encoding="utf-8")
    issues = {
        ("petersimmons1972/homelab-config", module.CODEx_LABEL): [],
        ("petersimmons1972/homelab-config", module.RUN_LABEL): [_build_issue(5, "trigger", ["agent/codex"], "2026-06-01T00:00:00Z", "petersimmons1972/homelab-config")],
        ("petersimmons1972/ollama", module.CODEx_LABEL): [],
        ("petersimmons1972/ollama", module.RUN_LABEL): [],
    }
    runner = FakeRunner(
        issues=issues,
        timelines={},
        pulls={
            "petersimmons1972/ollama": [
                {"head": {"ref": "agent/codex/merge-delete"}, "merged_at": "2026-06-01T00:00:00Z"},
            ],
        },
        pull_by_number={},
        branch_refs={("petersimmons1972/homelab-config", "agent/codex/"): [], ("petersimmons1972/ollama", "agent/codex/"): ["agent/codex/merge-delete"]},
        worktree_branches={},
    )
    cfg = _config(tmp_path, module, runner)
    (tmp_path / "projects" / "ollama").mkdir(parents=True, exist_ok=True)

    result = module.run_janitor(cfg, now=module.datetime(2026, 6, 7, tzinfo=module.timezone.utc))

    assert "petersimmons1972/ollama:agent/codex/merge-delete" in result.branches_pruned
    assert any(
        cmd[:2] == ["git", "-C"] and any("merge-delete" in segment for segment in cmd)
        for cmd in runner.calls
    )
