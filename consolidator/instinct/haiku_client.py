import json
import os
import asyncio
import anthropic

SYSTEM_PROMPT = """You are a pattern detection system analyzing Claude Code tool call sequences.

Analyze the tool call events and identify recurring patterns of these types:

1. CORRECTION: Evidence the user corrected the AI — re-do after rollback, "don't X" instruction, same action reversed within 3 steps.
2. ERROR_RESOLUTION: The same error (matching exit_status=1 + similar output_summary) followed by the same fix tool sequence, 2+ times.
3. WORKFLOW: A sequence of 3+ tool calls that recurs within the same session or across sessions in this batch.

Return a JSON array. Each pattern object must have these exact fields:
{
  "type": "correction" | "error_resolution" | "workflow",
  "description": "<human-readable pattern, one sentence, present tense>",
  "domain": "<one word: testing | git | editing | bash | agent | general>",
  "evidence": "<brief explanation of what you observed, max 100 chars>",
  "tag_signature": "<stable slug for deduplication, e.g. 'sig-edit-test-fail-edit'>"
}

If no patterns are found, return []. Return ONLY the JSON array — no prose, no markdown fences."""

MODEL = "claude-haiku-4-5-20251001"


def _build_user_message(events: list[dict]) -> str:
    lines = [
        f"[{e['timestamp']}] {e['tool_name']} | {e['tool_output_summary']} | exit={e['exit_status']}"
        for e in events
    ]
    return "Tool call events:\n" + "\n".join(lines)


def _valid_pattern(p: dict) -> bool:
    required = {"type", "description", "domain", "evidence", "tag_signature"}
    return required.issubset(p.keys()) and p["type"] in ("correction", "error_resolution", "workflow")


class HaikuClient:
    def __init__(self):
        self._client = anthropic.AsyncAnthropic(
            api_key=os.environ.get("ANTHROPIC_API_KEY", "")
        )

    async def detect(self, events: list[dict]) -> list[dict]:
        if not events:
            return []
        user_msg = _build_user_message(events)
        try:
            response = await self._client.messages.create(
                model=MODEL,
                max_tokens=1024,
                system=[
                    {
                        "type": "text",
                        "text": SYSTEM_PROMPT,
                        "cache_control": {"type": "ephemeral"},
                    }
                ],
                messages=[{"role": "user", "content": user_msg}],
            )
            raw = response.content[0].text.strip()
            patterns = json.loads(raw)
            if not isinstance(patterns, list):
                return []
            return [p for p in patterns if _valid_pattern(p)]
        except (json.JSONDecodeError, IndexError, anthropic.APIError):
            return []
