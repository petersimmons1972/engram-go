import json
import pytest
from pathlib import Path
from instinct.run import load_and_rotate_buffer, group_by_session


def make_events(n: int, session_id: str = "sess-a", project_id: str = "proj-001") -> list[dict]:
    return [
        {
            "timestamp": f"2026-04-22T10:0{i}:00Z",
            "session_id": session_id,
            "project_id": project_id,
            "tool_name": "Edit",
            "tool_input_hash": f"hash{i:04d}",
            "tool_output_summary": f"Edited file {i}",
            "exit_status": 0,
            "schema_version": 1,
        }
        for i in range(n)
    ]


def write_events(buffer: Path, events: list[dict]) -> None:
    with buffer.open("a") as f:
        for e in events:
            f.write(json.dumps(e) + "\n")


def test_load_and_rotate_returns_empty_when_buffer_missing(tmp_path):
    buffer = tmp_path / "buffer.jsonl"
    events = load_and_rotate_buffer(buffer)
    assert events == []


def test_load_and_rotate_returns_empty_when_fewer_than_20_events(tmp_path):
    buffer = tmp_path / "buffer.jsonl"
    write_events(buffer, make_events(5))
    events = load_and_rotate_buffer(buffer)
    assert events == []
    assert buffer.exists()  # buffer NOT rotated when noop


def test_load_and_rotate_rotates_buffer_when_20_plus_events(tmp_path):
    buffer = tmp_path / "buffer.jsonl"
    write_events(buffer, make_events(20))
    events = load_and_rotate_buffer(buffer)
    assert len(events) == 20
    assert not buffer.exists()  # rotated away
    processed = list(tmp_path.glob("buffer.jsonl.*.processed"))
    assert len(processed) == 1


def test_group_by_session_splits_correctly():
    events = make_events(3, session_id="s1") + make_events(2, session_id="s2")
    groups = group_by_session(events)
    assert len(groups) == 2
    assert len(groups[("s1", "proj-001")]) == 3
    assert len(groups[("s2", "proj-001")]) == 2


# ── run() integration tests ────────────────────────────────────────────────────

import os
import pytest
from unittest.mock import AsyncMock, MagicMock, patch


@pytest.mark.asyncio
async def test_run_noop_when_buffer_empty(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("INSTINCT_BUFFER", str(tmp_path / "buffer.jsonl"))
    monkeypatch.setenv("INSTINCT_MIN_EVENTS", "20")
    import importlib
    import instinct.run as run_mod
    importlib.reload(run_mod)
    await run_mod.run()
    out = capsys.readouterr().out
    assert "noop" in out


@pytest.mark.asyncio
async def test_run_processes_events_and_writes_episodes(tmp_path, monkeypatch, capsys):
    buf = tmp_path / "buffer.jsonl"
    write_events(buf, make_events(20))
    monkeypatch.setenv("INSTINCT_BUFFER", str(buf))
    monkeypatch.setenv("INSTINCT_MIN_EVENTS", "20")

    mock_engram = AsyncMock()
    mock_engram.__aenter__ = AsyncMock(return_value=mock_engram)
    mock_engram.__aexit__ = AsyncMock(return_value=False)
    mock_engram.write_episode = AsyncMock()
    mock_engram.query_pattern = AsyncMock(return_value=None)
    mock_engram.store_pattern = AsyncMock()

    mock_haiku = MagicMock()
    mock_haiku.detect = AsyncMock(return_value=[])

    import importlib
    import instinct.run as run_mod
    importlib.reload(run_mod)
    run_mod.BUFFER = buf

    with patch("instinct.run.EngramClient", return_value=mock_engram), \
         patch("instinct.run.HaikuClient", return_value=mock_haiku), \
         patch("instinct.run.upsert_pattern", new_callable=AsyncMock):
        await run_mod.run()

    out = capsys.readouterr().out
    assert "20 events" in out


@pytest.mark.asyncio
async def test_run_requeues_events_on_engram_error(tmp_path, monkeypatch, capsys):
    buf = tmp_path / "buffer.jsonl"
    write_events(buf, make_events(20))
    monkeypatch.setenv("INSTINCT_BUFFER", str(buf))
    monkeypatch.setenv("INSTINCT_MIN_EVENTS", "20")

    mock_engram_ctx = AsyncMock()
    mock_engram_ctx.__aenter__ = AsyncMock(side_effect=RuntimeError("engram down"))
    mock_engram_ctx.__aexit__ = AsyncMock(return_value=False)

    import importlib
    import instinct.run as run_mod
    importlib.reload(run_mod)
    run_mod.BUFFER = buf

    with patch("instinct.run.EngramClient", return_value=mock_engram_ctx), \
         patch("instinct.run.HaikuClient"):
        await run_mod.run()

    out = capsys.readouterr().out
    assert "ERROR" in out
    assert buf.exists()
