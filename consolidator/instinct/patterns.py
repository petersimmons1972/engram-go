CONFIDENCE_STEPS = [0.3, 0.5, 0.7, 0.9]
PROMOTE_THRESHOLD = 0.8


def next_confidence(current: float) -> float:
    for i, step in enumerate(CONFIDENCE_STEPS):
        if abs(current - step) < 0.01 and i + 1 < len(CONFIDENCE_STEPS):
            return CONFIDENCE_STEPS[i + 1]
    return CONFIDENCE_STEPS[-1]


def prev_confidence(current: float) -> float:
    for i, step in enumerate(CONFIDENCE_STEPS):
        if abs(current - step) < 0.01 and i > 0:
            return CONFIDENCE_STEPS[i - 1]
    return CONFIDENCE_STEPS[0]


async def upsert_pattern(engram, pattern: dict, events: list[dict]) -> None:
    project_ids = list({e["project_id"] for e in events})
    primary_project = project_ids[0]

    existing = await engram.query_pattern(pattern["tag_signature"], primary_project)

    if existing is None:
        await engram.store_pattern(pattern, 0.3, primary_project)
        return

    current_confidence = existing.get("importance", 0.3)
    if pattern.get("type") == "correction":
        new_confidence = prev_confidence(current_confidence)
    else:
        new_confidence = next_confidence(current_confidence)
    if new_confidence != current_confidence:
        await engram.update_confidence(existing["id"], new_confidence)

    # Promote to global if high confidence and seen across multiple projects
    if new_confidence >= PROMOTE_THRESHOLD and len(project_ids) >= 2:
        global_existing = await engram.query_pattern(pattern["tag_signature"], "global")
        if global_existing is None:
            await engram.store_pattern(pattern, new_confidence, "global")
