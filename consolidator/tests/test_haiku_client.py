import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from instinct.haiku_client import HaikuClient, _build_user_message

SAMPLE_EVENTS = [
    {
        "timestamp": "2026-04-22T10:00:00Z",
        "session_id": "sess-a",
        "project_id": "proj-001",
        "tool_name": "Edit",
        "tool_input_hash": "abc",
        "tool_output_summary": "Edited run.py",
        "exit_status": 0,
        "schema_version": 1,
    },
    {
        "timestamp": "2026-04-22T10:01:00Z",
        "session_id": "sess-a",
        "project_id": "proj-001",
        "tool_name": "Bash",
        "tool_input_hash": "def",
        "tool_output_summary": "pytest: 1 failed",
        "exit_status": 1,
        "schema_version": 1,
    },
    {
        "timestamp": "2026-04-22T10:02:00Z",
        "session_id": "sess-a",
        "project_id": "proj-001",
        "tool_name": "Edit",
        "tool_input_hash": "ghi",
        "tool_output_summary": "Edited run.py again",
        "exit_status": 0,
        "schema_version": 1,
    },
]


def test_build_user_message_includes_all_events():
    msg = _build_user_message(SAMPLE_EVENTS)
    assert "Edit" in msg
    assert "Bash" in msg
    assert len(msg) > 50


@pytest.mark.asyncio
async def test_haiku_client_returns_empty_list_on_no_patterns():
    with patch("instinct.haiku_client.anthropic.AsyncAnthropic") as mock_cls:
        mock_client = MagicMock()
        mock_cls.return_value = mock_client
        mock_client.messages.create = AsyncMock(
            return_value=MagicMock(content=[MagicMock(text="[]")])
        )
        client = HaikuClient()
        result = await client.detect(SAMPLE_EVENTS)
        assert result == []


@pytest.mark.asyncio
async def test_haiku_client_parses_valid_pattern_response():
    pattern_json = '[{"type":"correction","description":"Always run tests after Edit","domain":"testing","evidence":"Edit -> fail -> re-edit","tag_signature":"sig-abc123"}]'
    with patch("instinct.haiku_client.anthropic.AsyncAnthropic") as mock_cls:
        mock_client = MagicMock()
        mock_cls.return_value = mock_client
        mock_client.messages.create = AsyncMock(
            return_value=MagicMock(content=[MagicMock(text=pattern_json)])
        )
        client = HaikuClient()
        result = await client.detect(SAMPLE_EVENTS)
        assert len(result) == 1
        assert result[0]["type"] == "correction"
        assert result[0]["tag_signature"] == "sig-abc123"


@pytest.mark.asyncio
async def test_haiku_client_handles_malformed_response_gracefully():
    with patch("instinct.haiku_client.anthropic.AsyncAnthropic") as mock_cls:
        mock_client = MagicMock()
        mock_cls.return_value = mock_client
        mock_client.messages.create = AsyncMock(
            return_value=MagicMock(content=[MagicMock(text="not valid json {{{")])
        )
        client = HaikuClient()
        result = await client.detect(SAMPLE_EVENTS)
        assert result == []


@pytest.mark.asyncio
async def test_haiku_client_returns_empty_on_no_events():
    client = HaikuClient()
    result = await client.detect([])
    assert result == []
