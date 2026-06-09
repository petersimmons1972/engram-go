from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path
from typing import Any, Sequence


def _module():
    path = Path(__file__).resolve().parents[1] / "codex_digest.py"
    spec = importlib.util.spec_from_file_location("codex_digest", path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules["codex_digest"] = module
    spec.loader.exec_module(module)  # type: ignore[arg-type]
    return module


class FakeRunner:
    def __init__(self) -> None:
        self.calls: list[list[str]] = []
        self.responses: dict[tuple[str, ...], str] = {}

    def add_json(self, command: Sequence[str], payload: Any) -> None:
        self.responses[tuple(command)] = json.dumps(payload)

    def add_text(self, command: Sequence[str], payload: str) -> None:
        self.responses[tuple(command)] = payload

    def __call__(self, command: Sequence[str]) -> str:
        cmd = list(command)
        self.calls.append(cmd)
        return self.responses.get(tuple(cmd), "")


def test_render_digest_includes_all_sections_and_required_fields() -> None:
    module = _module()
    digest = module.render_digest(
        generated_at=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
        shipped=[
            module.DigestItem(
                repo="petersimmons1972/homelab-config",
                number=74,
                title="Add founder digest",
                url="https://github.com/petersimmons1972/homelab-config/issues/74",
                label="agent/codex/done",
                updated_at=module.parse_iso_datetime("2026-06-08T09:00:00Z"),
                pr_url="https://github.com/petersimmons1972/homelab-config/pull/101",
                pr_merged_at=module.parse_iso_datetime("2026-06-08T12:00:00Z"),
            )
        ],
        blocked=[
            module.DigestItem(
                repo="petersimmons1972/aifleet",
                number=313,
                title="Investigate watcher restart loop",
                url="https://github.com/petersimmons1972/aifleet/issues/313",
                label="agent/codex/in-progress",
                updated_at=module.parse_iso_datetime("2026-06-04T00:00:00Z"),
            )
        ],
        needs_you=[
            module.DigestItem(
                repo="petersimmons1972/olla",
                number=88,
                title="Decide rollout policy",
                url="https://github.com/petersimmons1972/olla/issues/88",
                label="decision/needs-founder",
                updated_at=module.parse_iso_datetime("2026-06-06T00:00:00Z"),
            )
        ],
        now=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
    )

    assert "## Shipped" in digest
    assert "## Blocked / in-flight" in digest
    assert "## Needs you" in digest
    assert "Add founder digest" in digest
    assert "https://github.com/petersimmons1972/homelab-config/pull/101" in digest
    assert "merged: 2026-06-08" in digest
    assert "Investigate watcher restart loop" in digest
    assert "label: agent/codex/in-progress" in digest
    assert "age: 5 days" in digest
    assert "Decide rollout policy" in digest
    assert "label: decision/needs-founder" in digest
    assert "https://github.com/petersimmons1972/olla/issues/88" in digest
    assert "age: 3 days" in digest


def test_render_digest_uses_unknown_age_when_needs_you_missing_updated_at() -> None:
    module = _module()
    digest = module.render_digest(
        generated_at=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
        shipped=[],
        blocked=[],
        needs_you=[
            module.DigestItem(
                repo="petersimmons1972/codex",
                number=28,
                title="Need founder input",
                url="https://github.com/petersimmons1972/codex/issues/28",
                label="agent/codex/needs-claude",
                updated_at=None,
            )
        ],
        now=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
    )

    assert "age: unknown days" in digest
    assert "Need founder input" in digest


def test_render_digest_renders_empty_sections() -> None:
    module = _module()
    digest = module.render_digest(
        generated_at=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
        shipped=[],
        blocked=[],
        needs_you=[],
        now=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
    )

    assert digest.count("_None this period._") == 3


def test_render_digest_stays_within_length_guardrail_for_small_sections() -> None:
    module = _module()
    now = module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc)
    shipped = [
        module.DigestItem(
            repo="petersimmons1972/homelab-config",
            number=index,
            title=f"Shipped {index}",
            url=f"https://github.com/petersimmons1972/homelab-config/issues/{index}",
            label="agent/codex/done",
            updated_at=now,
            pr_url=f"https://github.com/petersimmons1972/homelab-config/pull/{index}",
            pr_merged_at=now,
        )
        for index in range(1, 6)
    ]
    blocked = [
        module.DigestItem(
            repo="petersimmons1972/aifleet",
            number=index,
            title=f"Blocked {index}",
            url=f"https://github.com/petersimmons1972/aifleet/issues/{index}",
            label="agent/codex/blocked",
            updated_at=now,
        )
        for index in range(10, 15)
    ]
    needs_you = [
        module.DigestItem(
            repo="petersimmons1972/olla",
            number=index,
            title=f"Need {index}",
            url=f"https://github.com/petersimmons1972/olla/issues/{index}",
            label="decision/needs-founder",
            updated_at=now,
        )
        for index in range(20, 25)
    ]

    digest = module.render_digest(
        generated_at=now,
        shipped=shipped,
        blocked=blocked,
        needs_you=needs_you,
        now=now,
    )

    assert len(digest.splitlines()) <= 30


def test_post_digest_comments_on_tracking_issue(tmp_path: Path) -> None:
    module = _module()
    runner = FakeRunner()
    tracking_issue_file = tmp_path / "digest-tracking-issue.txt"
    tracking_issue_file.write_text("123\n", encoding="utf-8")

    module.post_digest(
        repo="petersimmons1972/claude-codex",
        tracking_issue_file=tracking_issue_file,
        digest_body="## Digest\n- item",
        runner=runner,
    )

    assert runner.calls == [
        [
            "gh",
            "issue",
            "comment",
            "123",
            "--repo",
            "petersimmons1972/claude-codex",
            "--body",
            "## Digest\n- item",
        ]
    ]


def test_ensure_tracking_issue_creates_and_records_issue_when_missing(tmp_path: Path) -> None:
    module = _module()
    runner = FakeRunner()
    tracking_issue_file = tmp_path / "digest-tracking-issue.txt"
    runner.add_json(
        [
            "gh",
            "api",
            "repos/petersimmons1972/claude-codex/issues",
            "-f",
            "title=Founder digest tracking",
            "-f",
            "body=Automated founder digest comments land here.",
            "-f",
            "labels[]=digest",
        ],
        {"number": 456},
    )

    issue_number = module.ensure_tracking_issue(
        repo="petersimmons1972/claude-codex",
        tracking_issue_file=tracking_issue_file,
        runner=runner,
    )

    assert issue_number == 456
    assert tracking_issue_file.read_text(encoding="utf-8").strip() == "456"


def test_done_issue_without_linked_merged_pr_is_omitted_from_shipped() -> None:
    module = _module()
    done_issue = module.DigestItem(
        repo="petersimmons1972/homelab-config",
        number=74,
        title="Done but not merged",
        url="https://github.com/petersimmons1972/homelab-config/issues/74",
        label="agent/codex/done",
        updated_at=module.parse_iso_datetime("2026-06-08T00:00:00Z"),
    )

    shipped = module.select_shipped_issues([done_issue])
    digest = module.render_digest(
        generated_at=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
        shipped=shipped,
        blocked=[],
        needs_you=[],
        now=module.datetime(2026, 6, 9, 13, 0, tzinfo=module.timezone.utc),
    )

    assert shipped == []
    assert "Done but not merged" not in digest
