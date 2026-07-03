# LongMemEval Campaign — Runbook (exact steps to run a wave)

**Companion:** `FINDINGS.md`. **Audience:** any agent continuing this work.
**Golden rule:** NO API keys. Generation = `claude --print` (Claude sub). GPT judging =
`codex exec -m gpt-5.4` (Codex sub). gpt-4o is unavailable; use gpt-5.4/medium.

## Fixed parameters (do not change without reason)
- **Variant:** LME-M. Dataset: `testdata/longmemeval/longmemeval_m_cleaned.json` (500 q).
- **Corpus:** already ingested as projects `lme-golden20260602-*` in the shared engram DB,
  `cleanup-policy=never` ⇒ **persists, never re-ingest.** run-id = `golden20260602`.
- **engram endpoint:** `https://engram.petersimmons.com`
- **engram api-key:** do NOT hardcode in committed files. Get from
  `results/wave0-rerun-haiku-20260603/RUN_STATUS.json` (`.command_line`) or the
  `engram` namespace secret.
- **recall-topk 100, context-topk 8, workers 8** (match all prior waves).
- **Flag env vars (server-side):** `ENGRAM_SESSION_NDCG_AGG=true` (session-DCG / LEVER-8),
  `ENGRAM_PREFERENCE_MMR=1` (preference-MMR / H-NEW-2).

## Build the run binary
```
cd ~/projects/engram-go
go build -o /tmp/longmemeval ./cmd/longmemeval
```
(The deployed image `3aabf50`/`:latest` already has the flag code — verify with
`kubectl exec -n engram deploy/engram-go -c engram-go -- /engram --help | grep ndcg`.)

## One wave = 6 steps
### 1. Enable flag(s) on prod (skip for a flags-OFF baseline)
```
kubectl set env deploy/engram-go -n engram ENGRAM_SESSION_NDCG_AGG=true   # and/or ENGRAM_PREFERENCE_MMR=1
kubectl rollout status deploy/engram-go -n engram --timeout=180s
kubectl get deploy engram-go -n engram -o jsonpath='{.status.readyReplicas}/{.spec.replicas}'
```
### 2. Generate (reuse corpus → copy ingest checkpoint so it SKIPS re-ingest)
```
OUT=results/<wave-name>; mkdir -p $OUT
cp results/wave0-rerun-haiku-20260603/checkpoint-ingest.jsonl $OUT/
setsid bash -c "/tmp/longmemeval run \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --url https://engram.petersimmons.com --api-key <KEY> \
  --run-id golden20260602 --out $OUT \
  --recall-topk 100 --context-topk 8 \
  --generation-model sonnet \  # sonnet|haiku|opus, all via claude --print (sub)
  --cleanup-policy=never --workers 8 </dev/null > /tmp/$OUT.log 2>&1" &
```
Expect first log line `run: 500 ingest entries loaded, 0 already done` (= no re-ingest).
Writes `$OUT/checkpoint-run.jsonl` (one row per q: hypothesis, question_id, retrieved_ids).
Haiku ~30-45 min, Sonnet ~50-75 min for 500 @ 8 workers.

### 3. REVERT flags immediately after generation (minimise prod blast radius)
```
kubectl set env deploy/engram-go -n engram ENGRAM_SESSION_NDCG_AGG- ENGRAM_PREFERENCE_MMR-
kubectl rollout status deploy/engram-go -n engram --timeout=180s
```

### 4. Build the judge bundle (join hypotheses + dataset gold)
```
python3 - <<PY
import json
ds={q['question_id']:q for q in json.load(open('testdata/longmemeval/longmemeval_m_cleaned.json'))}
o=open('<JUDGE_DIR>/bundle.jsonl','w')
for l in open('<OUT>/checkpoint-run.jsonl'):
    r=json.loads(l); q=ds[r['question_id']]
    o.write(json.dumps({'question_id':r['question_id'],'question_type':q['question_type'],
        'question':q['question'],'hypothesis':r.get('hypothesis',''),'gold_answer':q['answer']})+'\n')
o.close()
PY
```
Put `<JUDGE_DIR>` inside the codex worktree
`~/projects/.codex-poll-worktrees/engram-go-issue-1030-lme-judge-harness/results/<name>`
so `codex exec --cd` that worktree can read it.

### 5. Judge with gpt-5.4 (Codex sub, NO key). Judge = Codex's own reasoning.
```
WT=~/projects/.codex-poll-worktrees/engram-go-issue-1030-lme-judge-harness
setsid bash -c "codex exec --ephemeral -m gpt-5.4 -c model_reasoning_effort=\"medium\" \
  --cd $WT \"\$(cat /tmp/judge-prompt.txt)\" </dev/null > /tmp/judge.log 2>&1" &
```
**Judge prompt (verbatim rubric — keep identical across waves):**
```
Judge N LongMemEval hypotheses with YOUR OWN gpt-5.4 reasoning. Do NOT call any
API/scorer-url/olla/key. YOU are the judge.
INPUT (local): results/<name>/bundle.jsonl — rows {question_id,question_type,question,hypothesis,gold_answer}
RUBRIC per row, one label:
  CORRECT: hypothesis contains all key facts from gold answer, no contradictions. Extra correct context is fine.
  PARTIALLY_CORRECT: some key facts present, others missing or hedged; partial overlap.
  INCORRECT: key facts wrong, contradicted, or absent (even if topically related).
APPEND each verdict to results/<name>/checkpoint-score.jsonl as you go:
  {"question_id","question_type","score_label"}.
Then write results/<name>/score_report.json with per question_type AND overall:
  total, correct, partially_correct, incorrect, strict=CORRECT/total, lenient=(CORRECT+PARTIALLY_CORRECT)/total.
Do NOT modify code/git/PRs. Only write those two files.
```

### 6. Compute / verify per-type table
```
python3 - <<PY
import json,collections
a=collections.defaultdict(lambda:[0,0,0])
for l in open('<JUDGE_DIR>/checkpoint-score.jsonl'):
    r=json.loads(l); t=r['question_type']; x=r['score_label']
    a[t][0 if x=='CORRECT' else 1 if x=='PARTIALLY_CORRECT' else 2]+=1
tot=[0,0,0]
for t in sorted(a):
    c,p,i=a[t]; n=c+p+i; tot=[tot[j]+a[t][j] for j in range(3)]
    print(f'{t:28} strict={c/n*100:5.1f}% lenient={(c+p)/n*100:5.1f}% (C{c} P{p} I{i})')
c,p,i=tot; n=sum(tot); print(f'{"OVERALL":28} strict={c/n*100:5.1f}% lenient={(c+p)/n*100:5.1f}%')
PY
```

## Monitoring & gotchas (learned the hard way)
- **Use `ps -eo cmd | grep '[p]attern'`, NOT `pgrep`/`pkill`** — those return exit 1 in this
  sandbox and abort multi-line scripts. Kill by explicit PID from `ps`.
- **`codex exec` needs `</dev/null`** or it blocks on "Reading additional input from stdin".
  Launch with `setsid bash -c "... </dev/null > log 2>&1" &`.
- **`codex exec` buffers its log** — track progress by the incremental
  `checkpoint-score.jsonl` line count, not the log.
- **`codex exec` lingers idle after finishing** — kill it (`kill -9 <pid>` by `ps`).
- The `rmcp ... agentgateway AuthRequired` error at codex startup is harmless (an MCP server
  it doesn't need for judging).
- **Always revert flags** after a wave; verify `... | grep -c NDCG|MMR` = 0 and 3/3 ready.

## Known-good baselines to compare against
- Wave-0 (Haiku reader, flags off): GPT-5.4 judge **63.8% strict / 68.2% lenient**.
- Cross-reader comparisons are NOT apples-to-apples (partial-label inflation). Compare
  Sonnet arms only to a **Sonnet flags-off baseline** (run this next).

## Next steps (priority order)
1. **Sonnet flags-OFF baseline** (step 2 with no flags + sonnet) → anchors the Sonnet arms,
   confirms the partial-inflation artifact. THE outstanding run.
2. If a flag ever shows a target-type lift > ~7pp, **replicate 3×** to beat generation noise
   before believing it.
3. Real lever for single-session-preference: **ingest-time atomic preference extraction**
   (re-ingest wave), not retrieval re-ranking. (Gated; bigger build.)
4. For true competitor comparability you'd need a gpt-4o reader+judge — not available on
   current subscriptions; document the gpt-5.4 caveat on any external number.
