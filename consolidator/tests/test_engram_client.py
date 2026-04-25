import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from instinct.engram_client import EngramClient

SAMPLE_EVENTS = [
    {
        "timestamp": "2026-04-22T10:00:00Z",
        "session_id": "sess-aaa",
        "project_id": "abc123def456",
        "tool_name": "Edit",
        "tool_input_hash": "aabbcc",
        "tool_output_summary": "Edited run.py",
        "exit_status": 0,
        "schema_version": 1,
    }
]

SAMPLE_PATTERN = {
    "type": "correction",
    "description": "Always run tests after editing Python files",
    "domain": "testing",
    "evidence": "Observed 2 times: Edit → test failure → re-run",
    "tag_signature": "sig-abc123",
}


@pytest.fixture
def mock_session():
    session = AsyncMock()
    session.call_tool = AsyncMock(return_value=MagicMock(
        content=[MagicMock(text='{"id": "mem-001", "status": "stored"}')]
    ))
    return session


@pytest.mark.asyncio
async def test_write_episode_calls_episode_start_and_end(mock_session):
    client = EngramClient.__new__(EngramClient)
    client._session = mock_session

    await client.write_episode("sess-aaa", "proj-001", SAMPLE_EVENTS)

    calls = [c.args[0] for c in mock_session.call_tool.call_args_list]
    assert "memory_episode_start" in calls
    assert "memory_episode_end" in calls


@pytest.mark.asyncio
async def test_query_pattern_returns_none_when_not_found(mock_session):
    mock_session.call_tool = AsyncMock(return_value=MagicMock(
        content=[MagicMock(text='{"results": []}')]
    ))
    client = EngramClient.__new__(EngramClient)
    client._session = mock_session

    result = await client.query_pattern("sig-xyz", "proj-001")
    assert result is None


@pytest.mark.asyncio
async def test_store_pattern_calls_memory_store(mock_session):
    client = EngramClient.__new__(EngramClient)
    client._session = mock_session

    await client.store_pattern(SAMPLE_PATTERN, confidence=0.3, project_id="proj-001")

    call_args = mock_session.call_tool.call_args
    assert call_args.args[0] == "memory_store"
    kwargs = call_args.args[1]  # arguments dict
    assert kwargs["memory_type"] == "pattern"
    assert kwargs["importance"] == 0.3
    assert "instinct" in kwargs["tags"]
    assert "sig-abc123" in kwargs["tags"]


@pytest.mark.asyncio
async def test_update_confidence_calls_memory_correct(mock_session):
    client = EngramClient.__new__(EngramClient)
    client._session = mock_session

    await client.update_confidence("mem-001", 0.5)

    call_args = mock_session.call_tool.call_args
    assert call_args.args[0] == "memory_correct"
    kwargs = call_args.args[1]
    assert kwargs["memory_id"] == "mem-001"
    assert kwargs["importance"] == 0.5
