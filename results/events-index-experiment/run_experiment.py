#!/usr/bin/env python3
"""
Events-as-Index Retrieval Experiment
=====================================
Tests whether LLM-extracted object/entity indexes improve recall
for multi-session aggregation questions where embedding rank is poor.

Design:
- Deep-retrieval subset: multi-session items where at least one gold
  constituent session's first-chunk embedding rank > 15.
- Per item: LLM-extract objects from each candidate session (gold +
  top-60 by embedding), re-rank by question keyword overlap.
- Metric: gold recall@8/@15/@30 under embedding vs event-index ranking.
- GO if >=8 items lift ALL gold constituents into top-15 where embedding
  did NOT; NO-GO if <=4.
"""

import json
import sys
import time
import hashlib
import re
import string
import requests
from pathlib import Path
from collections import defaultdict
from typing import Dict, List, Optional, Set

# Paths
BASE_DIR = Path("/home/psimmons/projects/engram-go")
DATASET_PATH = BASE_DIR / "testdata/longmemeval/longmemeval_m_cleaned.json"
RUN_PATH = BASE_DIR / "results/covsynth-rep1-0621/checkpoint-run.jsonl"
INGEST_PATH = BASE_DIR / "results/covsynth-rep1-0621/checkpoint-ingest.jsonl"
OUT_DIR = BASE_DIR / "results/events-index-experiment"
CACHE_DIR = OUT_DIR / "cache"

OLLA_URL = "http://192.168.0.138:30411/olla/openai/v1/chat/completions"
OLLA_MODEL = "fast-inference"
MAX_SESSION_CHARS = 4000
EMBED_RANK_THRESHOLD = 15   # gold sessions with embedding rank > this are "deep"
CANDIDATE_TOPK = 60         # top-K unique sessions by embedding rank as candidates

STOP_ON_OLLA_ERRORS = 3


def log(msg):
    ts = time.strftime("%H:%M:%S")
    print(f"[{ts}] {msg}", flush=True)


def session_to_text(session_turns: List[dict]) -> str:
    parts = []
    for turn in session_turns:
        role = turn.get("role", "?")
        content = turn.get("content", "")
        parts.append(f"{role}: {content}")
    return "\n".join(parts)


def sha256_key(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:16]


def load_cache(key: str) -> Optional[dict]:
    p = CACHE_DIR / f"{key}.json"
    if p.exists():
        try:
            with open(p) as f:
                return json.load(f)
        except Exception:
            pass
    return None


def save_cache(key: str, data: dict):
    p = CACHE_DIR / f"{key}.json"
    with open(p, "w") as f:
        json.dump(data, f)


def extract_objects_llm(session_text: str) -> Optional[dict]:
    """Call olla to extract objects/entities and an event summary.
    Returns None only if all 3 attempts fail (caller tracks session-level failures).
    """
    truncated = session_text[:MAX_SESSION_CHARS]
    prompt = (
        "Extract from this conversation:\n"
        "1. Objects: a JSON array of normalized noun phrases for specific objects, topics, "
        "or entities that the user acted on, worked with, or discussed "
        "(e.g. [\"guitar\", \"Python script\", \"recipe\"]).\n"
        "2. A 1-sentence event_summary.\n\n"
        "Return ONLY valid JSON: {\"objects\": [...], \"event_summary\": \"...\"}\n\n"
        f"CONVERSATION:\n{truncated}"
    )
    payload = {
        "model": OLLA_MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 200,
        "temperature": 0.0,
    }
    content = ""
    for attempt in range(3):
        try:
            resp = requests.post(OLLA_URL, json=payload, timeout=90)
            resp.raise_for_status()
            content = resp.json()["choices"][0]["message"]["content"].strip()
            # Try to find JSON object in response
            m = re.search(r'\{.*\}', content, re.DOTALL)
            if m:
                parsed = json.loads(m.group())
                if "objects" in parsed:
                    return parsed
            parsed = json.loads(content)
            return parsed
        except requests.exceptions.RequestException as e:
            log(f"  Olla request error (attempt {attempt+1}/3): {e}")
            time.sleep(5)
        except json.JSONDecodeError:
            # LLM gave non-JSON - treat as empty extraction, don't retry
            return {"objects": [], "event_summary": content[:100]}
        except Exception as e:
            log(f"  Unexpected error (attempt {attempt+1}/3): {e}")
            time.sleep(5)
    return None


def normalize_words(text: str) -> Set[str]:
    text = text.lower()
    text = text.translate(str.maketrans(string.punctuation, ' ' * len(string.punctuation)))
    stops = {
        'the', 'a', 'an', 'and', 'or', 'but', 'in', 'on', 'at', 'to', 'for',
        'of', 'with', 'is', 'was', 'are', 'were', 'be', 'been', 'have', 'has',
        'had', 'do', 'does', 'did', 'that', 'this', 'it', 'its', 'i', 'you',
        'my', 'me', 'your', 'he', 'she', 'they', 'we', 'what', 'when', 'how',
        'which', 'who', 'about', 'from', 'can', 'will', 'would', 'could', 'should',
        'if', 'as', 'by', 'up', 'not', 'so', 'than', 'into', 'out', 'any', 'all',
    }
    return {w for w in text.split() if w not in stops and len(w) > 1}


def object_overlap_score(question_words: Set[str], session_objects: List[str]) -> float:
    if not session_objects:
        return 0.0
    all_obj_words: Set[str] = set()
    for obj in session_objects:
        all_obj_words |= normalize_words(obj)
    if not all_obj_words:
        return 0.0
    intersection = question_words & all_obj_words
    union = question_words | all_obj_words
    return len(intersection) / max(len(union), 1)


def recall_at_k(gold_ids: List[str], ranked: List[str], k: int) -> float:
    top_k = set(ranked[:k])
    return sum(1 for g in gold_ids if g in top_k) / max(len(gold_ids), 1)


def all_gold_in_topk(gold_ids: List[str], ranked: List[str], k: int) -> bool:
    top_k = set(ranked[:k])
    return all(g in top_k for g in gold_ids)


def main():
    log("=== Events-as-Index Retrieval Experiment ===")
    CACHE_DIR.mkdir(parents=True, exist_ok=True)

    log("Loading dataset...")
    with open(DATASET_PATH) as f:
        dataset = json.load(f)
    ds_by_qid = {item['question_id']: item for item in dataset}

    log("Loading run checkpoint...")
    with open(RUN_PATH) as f:
        run_lines = [json.loads(l) for l in f]

    log("Loading ingest checkpoint...")
    with open(INGEST_PATH) as f:
        ingest_lines = [json.loads(l) for l in f]

    ingest_by_qid: Dict[str, Dict[str, str]] = {
        ingest['question_id']: ingest.get('memory_map', {})
        for ingest in ingest_lines
    }

    log(f"Loaded {len(dataset)} dataset items, {len(run_lines)} run, {len(ingest_lines)} ingest")

    # --- Step 1: Identify deep-retrieval subset ---
    log(f"\nStep 1: Finding deep-retrieval subset (gold embed rank > {EMBED_RANK_THRESHOLD})...")

    deep_items = []
    for run in run_lines:
        qid = run['question_id']
        item = ds_by_qid.get(qid)
        if item is None:
            continue
        gold_session_ids = item.get('answer_session_ids', [])
        if len(gold_session_ids) <= 1:
            continue  # not multi-session

        retrieved_ids = run.get('retrieved_ids', [])
        memory_map = ingest_by_qid.get(qid, {})

        # Build session_id -> chunk UUID list
        session_to_uuids: Dict[str, List[str]] = defaultdict(list)
        for uuid, sid in memory_map.items():
            session_to_uuids[sid].append(uuid)

        uuid_rank = {uid: i for i, uid in enumerate(retrieved_ids)}

        gold_ranks = {}
        skip = False
        for gsid in gold_session_ids:
            uuids = session_to_uuids.get(gsid, [])
            if not uuids:
                skip = True
                break
            ranks = [uuid_rank.get(u, 9999) for u in uuids]
            gold_ranks[gsid] = min(ranks)

        if skip:
            continue

        max_gold_rank = max(gold_ranks.values())
        if max_gold_rank > EMBED_RANK_THRESHOLD:
            deep_items.append({
                'question_id': qid,
                'question': item['question'],
                'question_type': item['question_type'],
                'gold_session_ids': gold_session_ids,
                'gold_ranks': gold_ranks,
                'max_gold_rank': max_gold_rank,
                'retrieved_ids': retrieved_ids,
                'memory_map': memory_map,
                'haystack_session_ids': item['haystack_session_ids'],
                'haystack_sessions': item['haystack_sessions'],
            })

    log(f"Found {len(deep_items)} deep-retrieval items")

    if not deep_items:
        log("ERROR: No deep items found! Check threshold or data alignment.")
        sys.exit(1)

    with open(OUT_DIR / "deep_retrieval_subset.json", "w") as f:
        json.dump([
            {
                'question_id': d['question_id'],
                'question': d['question'][:120],
                'gold_session_ids': d['gold_session_ids'],
                'gold_ranks': d['gold_ranks'],
                'max_gold_rank': d['max_gold_rank'],
            }
            for d in deep_items
        ], f, indent=2)

    # --- Step 2: LLM extraction on candidate sessions ---
    log(f"\nStep 2: LLM extraction on candidate sessions (olla={OLLA_URL})...")

    consecutive_olla_failures = 0
    results = []
    total = len(deep_items)

    for item_idx, item in enumerate(deep_items):
        qid = item['question_id']
        question = item['question']
        gold_session_ids = item['gold_session_ids']
        retrieved_ids = item['retrieved_ids']
        memory_map = item['memory_map']
        haystack_session_ids = item['haystack_session_ids']
        haystack_sessions = item['haystack_sessions']

        log(f"\n[{item_idx+1}/{total}] qid={qid} | gold={gold_session_ids} | max_rank={item['max_gold_rank']}")

        # Build session_id -> text
        sid_to_text: Dict[str, str] = {}
        for sid, turns in zip(haystack_session_ids, haystack_sessions):
            sid_to_text[sid] = session_to_text(turns)

        # Build unique candidate list in embedding rank order
        seen_set: Set[str] = set()
        embedding_ranking: List[str] = []
        for uid in retrieved_ids:
            sid = memory_map.get(uid)
            if sid and sid not in seen_set:
                embedding_ranking.append(sid)
                seen_set.add(sid)
            if len(embedding_ranking) >= CANDIDATE_TOPK:
                break

        # Add gold sessions if not already present
        for gsid in gold_session_ids:
            if gsid not in seen_set:
                embedding_ranking.append(gsid)
                seen_set.add(gsid)

        log(f"  Candidates: {len(embedding_ranking)} sessions")

        question_words = normalize_words(question)

        # Extract objects for each candidate session, with cache
        session_extractions: Dict[str, dict] = {}
        new_count = 0
        cache_hits = 0

        for sid in embedding_ranking:
            text = sid_to_text.get(sid, "")
            if not text:
                session_extractions[sid] = {"objects": [], "event_summary": ""}
                continue

            key = sha256_key(text)
            cached = load_cache(key)
            if cached is not None:
                session_extractions[sid] = cached
                cache_hits += 1
                continue

            if consecutive_olla_failures >= STOP_ON_OLLA_ERRORS:
                log(f"  STOPPING: {consecutive_olla_failures} consecutive session-level olla failures")
                sys.exit(2)

            extraction = extract_objects_llm(text)
            if extraction is None:
                consecutive_olla_failures += 1
                log(f"  Extraction failed for {sid} (consecutive failures: {consecutive_olla_failures})")
                extraction = {"objects": [], "event_summary": ""}
            else:
                consecutive_olla_failures = 0  # reset on success

            save_cache(key, extraction)
            session_extractions[sid] = extraction
            new_count += 1

            if new_count % 5 == 0:
                n_cached = len(list(CACHE_DIR.glob("*.json")))
                log(f"  ... {new_count} new, {cache_hits} cached, {n_cached} total cache files")

        log(f"  Done: {new_count} new extractions, {cache_hits} cache hits")

        # Re-rank by event-index score (object overlap with question)
        scored = []
        for sid in embedding_ranking:
            ext = session_extractions.get(sid, {})
            score = object_overlap_score(question_words, ext.get("objects", []))
            scored.append((sid, score))
        scored.sort(key=lambda x: -x[1])
        event_index_ranking = [sid for sid, _ in scored]

        # Compute metrics
        metrics = {}
        for k in [8, 15, 30]:
            metrics[f"recall@{k}"] = {
                "embedding": recall_at_k(gold_session_ids, embedding_ranking, k),
                "event_index": recall_at_k(gold_session_ids, event_index_ranking, k),
                "embedding_all_gold": all_gold_in_topk(gold_session_ids, embedding_ranking, k),
                "event_index_all_gold": all_gold_in_topk(gold_session_ids, event_index_ranking, k),
            }

        m15 = metrics["recall@15"]
        log(f"  recall@15: embed={m15['embedding']:.2f} evt={m15['event_index']:.2f} | "
            f"all_gold_emb={m15['embedding_all_gold']} all_gold_evt={m15['event_index_all_gold']}")

        results.append({
            "question_id": qid,
            "question": question[:120],
            "gold_session_ids": gold_session_ids,
            "gold_embed_ranks": item['gold_ranks'],
            "max_gold_embed_rank": item['max_gold_rank'],
            "n_candidates": len(embedding_ranking),
            "metrics": metrics,
        })

    # --- Step 3: Aggregate ---
    log("\n=== AGGREGATE RESULTS ===")
    n = len(results)
    log(f"Total deep-retrieval items evaluated: {n}")

    lifted_into_15 = sum(
        1 for r in results
        if r["metrics"]["recall@15"]["event_index_all_gold"]
        and not r["metrics"]["recall@15"]["embedding_all_gold"]
    )
    embed_all15 = sum(1 for r in results if r["metrics"]["recall@15"]["embedding_all_gold"])
    evt_all15 = sum(1 for r in results if r["metrics"]["recall@15"]["event_index_all_gold"])

    log(f"Embedding: ALL gold in top-15 for {embed_all15}/{n} items")
    log(f"Event-index: ALL gold in top-15 for {evt_all15}/{n} items")
    log(f"LIFTED by event-index (not in emb top-15 → in evt top-15): {lifted_into_15}/{n}")

    for k in [8, 15, 30]:
        emb_avg = sum(r["metrics"][f"recall@{k}"]["embedding"] for r in results) / max(n, 1)
        evt_avg = sum(r["metrics"][f"recall@{k}"]["event_index"] for r in results) / max(n, 1)
        log(f"  avg recall@{k}: embed={emb_avg:.3f}  event={evt_avg:.3f}  delta={evt_avg - emb_avg:+.3f}")

    verdict = "GO" if lifted_into_15 >= 8 else ("NO-GO" if lifted_into_15 <= 4 else "BORDERLINE")
    log(f"\nVERDICT: {verdict} (GO>=8, NO-GO<=4, N={n})")

    output = {
        "experiment": "events-as-index",
        "n_deep_items": n,
        "embed_all_gold_in_top15": embed_all15,
        "event_index_all_gold_in_top15": evt_all15,
        "lifted_into_top15": lifted_into_15,
        "verdict": verdict,
        "total_cache_files": len(list(CACHE_DIR.glob("*.json"))),
        "items": results,
    }
    out_path = OUT_DIR / "results.json"
    with open(out_path, "w") as f:
        json.dump(output, f, indent=2)
    log(f"Results saved to {out_path}")
    log("=== DONE ===")


if __name__ == "__main__":
    main()
