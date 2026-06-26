import json
f='/home/psimmons/projects/engram-go/results/events-index-experiment/results.json'
d=json.load(open(f))
items=d['items']

print("=== FILE HEADLINE (experiment's own) ===")
for k in ['experiment','n_deep_items','embed_all_gold_in_top15','event_index_all_gold_in_top15','lifted_into_top15','verdict','total_cache_files']:
    print(f"  {k}: {d.get(k)}")

print("\n=== PRODUCTION-MEANINGFUL FILTER (pool-eligible: all gold embed-rank <= 60) ===")
POOL=60
CUT='recall@15'
elig=[it for it in items if it['max_gold_embed_rank'] <= POOL]
print(f"  pool-eligible items (max_gold_embed_rank<=60): {len(elig)} of {len(items)}")

embed_allgold = sum(1 for it in elig if it['metrics'][CUT]['embedding_all_gold'])
evt_allgold   = sum(1 for it in elig if it['metrics'][CUT]['event_index_all_gold'])
print(f"  [{CUT}] embedding gets ALL gold in top15:   {embed_allgold}/{len(elig)}")
print(f"  [{CUT}] event-index gets ALL gold in top15: {evt_allgold}/{len(elig)}   <-- GO metric (threshold >=8)")
lift = evt_allgold - embed_allgold
print(f"  net lift from event-index (eligible): {lift:+d}")

# items event-index lifts that embedding missed
lifted=[it['question_id'] for it in elig
        if it['metrics'][CUT]['event_index_all_gold'] and not it['metrics'][CUT]['embedding_all_gold']]
dropped=[it['question_id'] for it in elig
         if it['metrics'][CUT]['embedding_all_gold'] and not it['metrics'][CUT]['event_index_all_gold']]
print(f"  lifted (evt yes / embed no): {lifted}")
print(f"  regressed (embed yes / evt no): {dropped}")

print("\n=== VERDICT ===")
go = evt_allgold >= 8
print(f"  pool-eligible event-index all-gold@15 = {evt_allgold}  vs threshold 8  ->  {'GO' if go else 'NO-GO' if evt_allgold<6 else 'MARGINAL'}")
print("  (LEXICAL LOWER BOUND — re-rank is Jaccard, not semantic; real semantic re-rank would do >= this)")
