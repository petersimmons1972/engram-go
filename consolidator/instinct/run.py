"""Buffer read, rotate, session grouping, and dispatch to engram + pattern detection."""

import asyncio
import json
import os
from datetime import datetime, timezone
from pathlib import Path

from instinct.engram_client import EngramClient
from instinct.haiku_client import HaikuClient
from instinct.patterns import upsert_pattern

BUFFER = Path(os.environ.get("INSTINCT_BUFFER", "~/.local/state/instinct/buffer.jsonl")).expanduser()
MIN_EVENTS = int(os.environ.get("INSTINCT_MIN_EVENTS", "20"))


def load_and_rotate_buffer(buffer_path: Path) -> list[dict]:
    """Load raw events from buffer JSONL and rotate (move) the file if >= MIN_EVENTS.

    Returns:
        Empty list if buffer missing or has < MIN_EVENTS events.
        Full list of parsed events if >= MIN_EVENTS and rotation succeeds.

    Side effect: On success, renames buffer_path to buffer.jsonl.YYYYMMDDTHHMMSSZ.processed
    """
    if not buffer_path.exists():
        return []

    raw_lines = [l.strip() for l in buffer_path.read_text().splitlines() if l.strip()]

    if len(raw_lines) < MIN_EVENTS:
        return []

    events = []
    skipped = 0
    for line in raw_lines:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            skipped += 1

    if skipped:
        print(f"instinct: WARN — {skipped} malformed line(s) skipped in {buffer_path.name}")

    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    dest = buffer_path.parent / f"buffer.jsonl.{ts}.processed"
    buffer_path.rename(dest)

    return events


def group_by_session(events: list[dict]) -> dict[tuple[str, str], list[dict]]:
    """Group events by (session_id, project_id) tuple.

    Args:
        events: List of raw event dicts, each containing session_id and project_id.

    Returns:
        Dict mapping (session_id, project_id) -> list of events in that group.
    """
    groups: dict[tuple[str, str], list[dict]] = {}
    for event in events:
        key = (event["session_id"], event["project_id"])
        groups.setdefault(key, []).append(event)
    return groups


async def run():
    """Main entry point: load buffer, rotate, group by session, write episodes, detect patterns."""
    events = load_and_rotate_buffer(BUFFER)
    if not events:
        print(f"instinct: noop ({BUFFER} has < {MIN_EVENTS} events or is missing)")
        return

    groups = group_by_session(events)
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    print(f"instinct: {len(events)} events across {len(groups)} sessions [{ts}]")

    # Capture the rotated file path so we can restore it if Engram fails.
    processed_files = sorted(BUFFER.parent.glob("buffer.jsonl.*.processed"), reverse=True)
    processed_path = processed_files[0] if processed_files else None

    try:
        async with EngramClient() as engram:
            for (session_id, project_id), group_events in groups.items():
                await engram.write_episode(session_id, project_id, group_events)
                print(f"instinct: wrote episode for {session_id[:8]}.. @ {project_id}")

            haiku = HaikuClient()
            patterns = await haiku.detect(events)
            print(f"instinct: detected {len(patterns)} patterns [{ts}]")

            for pattern in patterns:
                await upsert_pattern(engram, pattern, events)

        print(f"instinct: processed {len(events)} events, found {len(patterns)} patterns [{ts}]")

    except Exception as exc:
        # Re-queue events: rename .processed back to buffer.jsonl so the next
        # run can pick them up.  Only do this if the buffer file isn't already
        # present (another process could have started a new buffer in the gap).
        print(f"instinct: ERROR — {exc}. Re-queuing {len(events)} events.")
        if processed_path and processed_path.exists() and not BUFFER.exists():
            processed_path.rename(BUFFER)


def cli():
    """Console entry point for instinct run command."""
    asyncio.run(run())


if __name__ == "__main__":
    cli()
