"""Engram MCP client for writing instinct episodes and patterns.

Wraps the MCP memory_* tools over SSE transport. Use as an async context
manager for managed sessions, or inject a pre-built session (as tests do)
directly onto client._session.
"""

import json
from datetime import date
from pathlib import Path
from mcp import ClientSession
from mcp.client.sse import sse_client


class EngramClient:
    def __init__(self, config_path: str | None = None):
        self._config_path = config_path or str(Path("~/.claude/mcp_servers.json").expanduser())
        self._session: ClientSession | None = None
        self._stack = None

    def _get_config(self) -> tuple[str, str]:
        """Read SSE URL and Bearer token from mcp_servers.json."""
        config_path = Path(self._config_path)
        if not config_path.exists():
            raise RuntimeError(
                f"Engram MCP config not found at {config_path}. "
                "Run `claude mcp add engram` or create the file manually."
            )
        try:
            cfg = json.loads(config_path.read_text())
            engram = cfg["mcpServers"]["engram"]
            url = engram["url"]
            auth = engram["headers"]["Authorization"]
        except (KeyError, json.JSONDecodeError) as exc:
            raise RuntimeError(
                f"Engram MCP server not configured in {config_path}. "
                "Expected: mcpServers.engram.url and mcpServers.engram.headers.Authorization"
            ) from exc
        return url, auth

    async def __aenter__(self):
        from contextlib import AsyncExitStack
        url, auth = self._get_config()
        self._stack = AsyncExitStack()
        read, write = await self._stack.enter_async_context(
            sse_client(url, headers={"Authorization": auth})
        )
        self._session = await self._stack.enter_async_context(ClientSession(read, write))
        await self._session.initialize()
        return self

    async def __aexit__(self, *args):
        await self._stack.aclose()

    async def _call(self, tool_name: str, arguments: dict) -> dict:
        """Call an MCP tool and return the parsed JSON response."""
        result = await self._session.call_tool(tool_name, arguments)
        if result.content:
            try:
                return json.loads(result.content[0].text)
            except (json.JSONDecodeError, IndexError):
                return {}
        return {}

    async def write_episode(self, session_id: str, project_id: str, events: list[dict]) -> None:
        """Write a batch of raw tool events as a single Engram episode.

        Opens an episode, ingests each event as a context memory, then
        closes the episode. The episode groups all events under one title
        so they can be recalled or aggregated together.
        """
        resp = await self._call(
            "memory_episode_start",
            {"title": f"instinct-raw:{session_id}", "project": project_id},
        )
        # Engram may return the ID under "episode_id" or "id" depending on version.
        episode_id = resp.get("episode_id") or resp.get("id", "")

        for event in events:
            await self._call(
                "memory_ingest",
                {
                    "content": json.dumps(event),
                    "memory_type": "context",
                    "project": project_id,
                    "importance": 0.2,
                    "tags": ["instinct-raw", f"session-{session_id}"],
                },
            )

        # Always close the episode — the server opened one on episode_start
        # regardless of whether the ID came back in the expected field.
        await self._call("memory_episode_end", {"episode_id": episode_id})

    async def query_pattern(self, tag_signature: str, project_id: str) -> dict | None:
        """Look up an existing pattern by its tag signature.

        Returns the first result whose tags include tag_signature, or None
        if no match is found. Used by the pattern engine to check for
        duplicates before storing a new pattern.
        """
        resp = await self._call(
            "memory_recall",
            {"query": f"instinct pattern {tag_signature}", "project": project_id},
        )
        results = resp.get("results", [])
        for r in results:
            if tag_signature in r.get("tags", []):
                return r
        return None

    async def store_pattern(self, pattern: dict, confidence: float, project_id: str) -> str:
        """Persist a new behaviour pattern at the given confidence level.

        Returns the Engram memory ID of the stored record, or '' if the
        response did not include one.
        """
        content = (
            f"{pattern['description']} | "
            f"PROVENANCE: observed 1 time, first seen {date.today().isoformat()}"
        )
        resp = await self._call(
            "memory_store",
            {
                "content": content,
                "memory_type": "pattern",
                "project": project_id,
                "importance": confidence,
                "tags": [
                    "instinct",
                    pattern["type"],
                    pattern.get("domain", "general"),
                    pattern["tag_signature"],
                ],
            },
        )
        return resp.get("id", "")

    async def update_confidence(self, memory_id: str, new_confidence: float) -> None:
        """Revise the importance/confidence of an existing pattern record."""
        await self._call(
            "memory_correct",
            {
                "memory_id": memory_id,
                "correction": f"confidence updated to {new_confidence}",
                "importance": new_confidence,
            },
        )
