import pytest
from unittest.mock import AsyncMock
from instinct.patterns import upsert_pattern, next_confidence, prev_confidence

SAMPLE_PATTERN = {
    "type": "workflow",
    "description": "Always run tests after Edit",
    "domain": "testing",
    "evidence": "Edit -> fail -> re-edit",
    "tag_signature": "sig-edit-test",
}

SAMPLE_EVENTS = [
    {
        "session_id": "s1",
        "project_id": "p1",
        "tool_name": "Edit",
        "exit_status": 0,
        "timestamp": "2026-04-22T10:00:00Z",
        "tool_input_hash": "x",
        "tool_output_summary": "",
        "schema_version": 1,
    },
]


def test_next_confidence_steps_up():
    assert next_confidence(0.3) == 0.5
    assert next_confidence(0.5) == 0.7
    assert next_confidence(0.7) == 0.9
    assert next_confidence(0.9) == 0.9  # capped


def test_prev_confidence_steps_down():
    assert prev_confidence(0.9) == 0.7
    assert prev_confidence(0.5) == 0.3
    assert prev_confidence(0.3) == 0.3  # floored


@pytest.mark.asyncio
async def test_upsert_pattern_stores_new_when_not_found():
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(return_value=None)
    engram.store_pattern = AsyncMock(return_value="mem-new")

    await upsert_pattern(engram, SAMPLE_PATTERN, SAMPLE_EVENTS)

    engram.store_pattern.assert_called_once_with(SAMPLE_PATTERN, 0.3, "p1")


@pytest.mark.asyncio
async def test_upsert_pattern_raises_confidence_when_found():
    existing = {"id": "mem-001", "importance": 0.3, "tags": ["sig-edit-test"]}
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(return_value=existing)
    engram.update_confidence = AsyncMock()

    await upsert_pattern(engram, SAMPLE_PATTERN, SAMPLE_EVENTS)

    engram.update_confidence.assert_called_once_with("mem-001", 0.5)


@pytest.mark.asyncio
async def test_upsert_pattern_no_update_at_max_confidence():
    """When importance is already at the ceiling (0.9), update_confidence must not be called."""
    existing = {"id": "mem-001", "importance": 0.9, "tags": ["sig-edit-test"]}
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(return_value=existing)
    engram.update_confidence = AsyncMock()

    await upsert_pattern(engram, SAMPLE_PATTERN, SAMPLE_EVENTS)

    engram.update_confidence.assert_not_called()


@pytest.mark.asyncio
async def test_upsert_pattern_promotes_to_global_at_high_confidence():
    existing = {"id": "mem-001", "importance": 0.7, "tags": ["sig-edit-test"]}
    # Two different project_ids
    events = [
        {**SAMPLE_EVENTS[0], "project_id": "p1"},
        {**SAMPLE_EVENTS[0], "project_id": "p2"},
    ]
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(side_effect=[existing, None])  # found in p1, not in global
    engram.update_confidence = AsyncMock()
    engram.store_pattern = AsyncMock(return_value="mem-global")

    await upsert_pattern(engram, SAMPLE_PATTERN, events)

    # Should update p1's confidence to 0.9 AND store a global copy
    engram.update_confidence.assert_called_once_with("mem-001", 0.9)
    engram.store_pattern.assert_called_once_with(SAMPLE_PATTERN, 0.9, "global")


@pytest.mark.asyncio
async def test_upsert_pattern_demotes_existing_correction():
    """Correction patterns that already exist should step DOWN in confidence."""
    correction = {**SAMPLE_PATTERN, "type": "correction", "tag_signature": "sig-correction-1"}
    existing = {"id": "mem-002", "importance": 0.7, "tags": ["sig-correction-1"]}
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(return_value=existing)
    engram.update_confidence = AsyncMock()

    await upsert_pattern(engram, correction, SAMPLE_EVENTS)

    engram.update_confidence.assert_called_once_with("mem-002", 0.5)


@pytest.mark.asyncio
async def test_upsert_correction_at_floor_does_not_call_update():
    """Correction already at minimum confidence (0.3) — no update needed."""
    correction = {**SAMPLE_PATTERN, "type": "correction", "tag_signature": "sig-floor"}
    existing = {"id": "mem-003", "importance": 0.3, "tags": ["sig-floor"]}
    engram = AsyncMock()
    engram.query_pattern = AsyncMock(return_value=existing)
    engram.update_confidence = AsyncMock()

    await upsert_pattern(engram, correction, SAMPLE_EVENTS)

    engram.update_confidence.assert_not_called()
