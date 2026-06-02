# LME Golden-Fixture Build Lessons (run golden20260602, 2026-06-02)

This document captures hard-won operational lessons from building the full-corpus
LongMemEval golden fixture (run-id `golden20260602`) using the 500-question "m"
corpus. It is an operational runbook and lessons record for the next person who
attempts a full-corpus golden build.

Binary: `/tmp/lme-golden-build/longmemeval` (built from main `98a4d5d`)  
Run-id: `golden20260602`  
Corpus: `testdata/longmemeval/longmemeval_m_cleaned.json` (500 questions)

---

## 1. Scale — you have ~2.1 M chunks, not ~237 K

The intuitive estimate is "500 questions × ~475 sessions/question = ~237 K
sessions ingested." That number is the **session** count. Ingest operates at
**turn** level. Each session averages ~9 turns, so the real landing numbers are:

| Metric | Value |
|--------|-------|
| Sessions | ~237,500 |
| Chunks (turns) | ~2,116,925 |
| Memories | ~2,116,882 |
| Chunk : Memory ratio | ≈ 1 : 1 |

**Size all embed-time and DB capacity estimates for ~2.1 M, not ~237 K.**
Underestimating by ~9× is the single most disorienting mistake of this campaign.

---

## 2. Pipeline order — ingest → WAIT for embed drain → run → score → merge

The correct pipeline is strictly ordered:

```
ingest (4 shards, same --run-id)
  ↓
[WAIT — embed drain, see §3 and §4]
  ↓
run (4 shards, --no-exclusive-backend --enable-thinking)
  ↓
score (4 shards)
  ↓
merge results
```

**Do not start the run stage until embed drain is complete.**

Ingest is async-embed: memories land in the DB almost immediately (fast path),
but embeddings are written by the background `reembed` worker (slow path).
If you launch `run` before embeddings are present, recall degrades to BM25-only
(issue #917), poisoning the fixture. There is no recovery short of re-running.

Sharding flags used:

```
--scratch-ttl 0 --cleanup-policy never
```

`--scratch-ttl 0` disables TTL expiry on scratch memories.
`--cleanup-policy never` ensures memories persist across the build.

---

## 3. Embed drain is the long pole — budget 12–18 hours

With the 500-question "m" corpus (~2.1 M embeddings), bge-m3 on 2 ROCm
endpoints (Precision W6800 + MI50), throughput was ~2,000 embeddings/min at
optimal concurrency. That gives:

```
2,116,925 embeddings ÷ 2,000/min ≈ 1,058 min ≈ 17.6 h
```

**Embed drain dominates wall-clock time, not ingest.** Ingest itself completes
in ~2–4 hours. The embed drain is ~5–9× longer. Plan accordingly; do not
schedule run/score until the drain has completed (§4).

---

## 4. Measure drain with psql — not cadence, not time

Cadence-based and time-based estimates of embed progress were wrong by up to
**~15×** during this campaign. The only reliable signal is a direct count of
un-embedded chunks in Postgres.

**Command (run inside the `engram-reembed` pod or any pod with DB access):**

```bash
psql -h trunas.petersimmons.com -p 5434 -U engram -d engram \
  -c "SELECT count(*) FROM chunks WHERE embedding IS NULL AND project LIKE 'lme-golden20260602-%';"
```

Substitute the run-id prefix for other runs. Poll this every few minutes and
wait for `count = 0` before advancing to the run stage. Do not trust the
reembed worker's log cadence or wall-clock extrapolation.

---

## 5. "Stall" diagnosis — sample before panicking

During this campaign, the ingest process appeared to stall (no visible progress
for several minutes). The actual cause was a slow large-item batch, not a hang.
The ingest completed during the diagnostic window.

**Before declaring a stall:**

1. Confirm the process is still alive (`ps`/`kill -0 <pid>`).
2. Confirm there are no error log lines.
3. Sample the checkpoint file line count at t=0, t=45s, t=90s.
4. Only declare a hang if count is static across all three samples **and**
   log output is absent for >90 s.

Large batches (long conversation threads) can produce 30–120 s of silence
between checkpoints. Silence alone is not a stall.

---

## 6. Concurrency vs backends — match, don't exceed

Raising `CONCURRENCY_MAX` above the number of working embed backends is
counterproductive. Measured throughput during this campaign:

| CONCURRENCY_MAX | Backends routable | Throughput |
|-----------------|-------------------|------------|
| 3 | 2 | ~1,333 embed/min |
| 2 | 2 | ~2,081 embed/min |

At concurrency=3 with 2 backends, the third slot introduces contention and
head-of-line blocking, reducing throughput by ~36%. **Set `CONCURRENCY_MAX`
equal to the number of actually-routable embed endpoints.**

---

## 7. Adding embed capacity is blocked by olla bug #73

We attempted to bring Leviathan online as a third ROCm embed backend to reduce
drain time. It was healthy (rocm-v0.5 image), latency ~348 ms, and responded
correctly to direct probes. **olla still would not route to it.**

Root cause: olla bug [petersimmons1972/olla#73](https://github.com/petersimmons1972/olla/issues/73).
olla poisons its HTTP transport for FC-discovered endpoints that were first seen
offline. Once poisoned, the endpoint flips `healthy → offline` on connection
reuse, even after recovery. Higher-latency hosts like Leviathan are particularly
susceptible. Restarting olla does not durably fix it.

**The image version (rocm-v0.5) was not the cause.** We confirmed correct image,
correct health response, correct direct embed — olla's transport layer was the
blocker.

Until #73 is fixed, you cannot reliably scale embed drain by adding endpoints.
This is the current capacity ceiling. Note it before planning any timeline that
assumes >2 active backends.

---

## Shard Command Templates

All four shards used the same binary, run-id, and flags. Only `--start`/`--end`
and output directory changed.

### Ingest (all 4 shards)

```bash
# Shard 1 (items 1–125)
/tmp/lme-golden-build/longmemeval ingest \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --run-id golden20260602 \
  --start 1 --end 125 \
  --scratch-ttl 0 --cleanup-policy never \
  --out results/lme-golden-20260602/shard1 \
  --url https://engram.petersimmons.com \
  --api-key <engram-api-key>

# Shard 2 (items 126–250)  --start 126 --end 250  --out .../shard2
# Shard 3 (items 251–375)  --start 251 --end 375  --out .../shard3
# Shard 4 (items 376–500)  --start 376 --end 500  --out .../shard4
```

### Run (after embed drain confirmed at 0)

```bash
# Shard 1
/tmp/lme-golden-build/longmemeval run \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --run-id golden20260602 \
  --start 1 --end 125 \
  --out results/lme-golden-20260602/shard1 \
  --backend http://oblivion:8000 \
  --no-exclusive-backend --enable-thinking \
  --url https://engram.petersimmons.com \
  --api-key <engram-api-key>

# Shards 2–4: same flags, adjust --start/--end/--out
```

### Score

```bash
# Shard 1
/tmp/lme-golden-build/longmemeval score \
  --out results/lme-golden-20260602/shard1 \
  --backend http://oblivion:8000 \
  --no-exclusive-backend

# Shards 2–4: adjust --out only
```

---

## Next-Time Checklist

Use this before starting a full-corpus golden build.

- [ ] **Size for 2.1 M** — not ~237 K. Confirm DB has capacity (Postgres +
      pgvector index). bge-m3 vectors are 1024-dim floats (~4 KB/embedding).
- [ ] **Budget ~12–18 h embed drain** — block run/score until drain is 0.
      Schedule accordingly; do not plan run/score for the same day as ingest.
- [ ] **Gate run on drain** — poll psql (§4 command) until count = 0. Do not
      use time estimates or log cadence.
- [ ] **Measure via psql** — the only reliable drain signal is
      `SELECT count(*) FROM chunks WHERE embedding IS NULL AND project LIKE 'lme-<runid>-%'`.
- [ ] **Match CONCURRENCY_MAX to routable backend count** — 2 backends → set
      `CONCURRENCY_MAX=2`. Exceeding it reduces throughput.
- [ ] **Watch olla #73** — if adding embed backends, verify olla is actually
      routing to them (direct probe + olla route list). Do not assume online =
      routable. Check if #73 is fixed before planning multi-backend scale-up.
- [ ] **Stall patience** — silence up to ~120 s between checkpoint lines is
      normal for large-batch items. Sample 3× over 90 s before treating as hung.
- [ ] **Use `--scratch-ttl 0 --cleanup-policy never`** on ingest shards to
      prevent memories expiring before run/score complete.
