# Issue #1345: `IsAggregationQuestion` divergence analysis

## Executive conclusion

On the intended LongMemEval-S **multi-session** cohort (133 questions), the reported counts reproduce exactly: engram-go classifies **91**, while duramind classifies **79**. The symmetric difference contains **12 questions, all engram-go-only**; duramind has no unique positives in this cohort.

Recommendation: make the engram-go behavior canonical, but describe and structure it as a **multi-fact composition** predicate rather than a literal counting-only predicate. Its 12 extra positives all require combining monetary facts; five are unambiguous sums/allocations and seven are differences or derived amounts that are borderline only under a narrow linguistic definition of “aggregation.” None is a duramind win for the operational purpose stated by both packages—detecting questions that require population-level recall. Preserve engram-go's temporal-duration exclusion, which prevents clear false positives outside the multi-session cohort.

Judgment tally for the 12 disagreements:

| Outcome | Count |
|---|---:|
| engram-go right | 5 |
| duramind right | 0 |
| genuinely ambiguous | 7 |

“Ambiguous” here means the question requires cross-fact arithmetic and should receive the same recall/composition treatment, but is a difference, rate, or derived value rather than a sum or count. If the canonical contract is operational (“needs broad recall plus composition”), all 12 are engram-go wins. If it is taxonomic (“is literally counting/summing a population”), the seven derived-value items should be a neighboring `composition` class rather than aggregation.

## Scope and reproducibility

Implementations inspected:

- engram-go: `internal/aggq/aggq.go`, lines 32–59, at commit `e45ac27446b6ff85c5d8c651e3ca02aae4c3239b`.
- duramind: `/home/psimmons/projects/duramind/internal/query/aggregation.go`, lines 17–25, at commit `3bfc653600569cfb4cb386391291ee9eb03a1ce8`.
- Dataset: `/home/psimmons/projects/engram-go/testdata/longmemeval/longmemeval_s.json`, SHA-256 `220b15c383dea8443105d43a78de47e43938386a6c91695ee130615a452dd091`.

The JSON contains 500 total questions. The issue's 91/79 figures arise after filtering to `question_type == "multi-session"` (133 questions). Applying both predicates to all 500 records instead yields 174 for engram-go and 185 for duramind; that whole-file result is not the comparison described by the issue because temporal-reasoning and single-session items are then included.

Reproduction method: load the JSON, retain records whose `question_type` is `multi-session`, evaluate the exact regular expressions and control flow shown below against each `question`, and compare the resulting `question_id` sets. This produces:

```text
cohort size:          133
engram-go positives:  91
duramind positives:   79
intersection:         79
engram-go only:       12
duramind only:         0
union:                91
```

## Logic diff

Both implementations match these case-insensitive, word-bounded phrases:

```text
how many | how often | how much total | total number of | sum of | count of
```

Engram-go adds three behaviors:

1. Additional exhaustive-list wording in its main expression: `list all`, `list every`, `list everything`, `every time`, and `all occasion(s)`. The `how many(?: times)?` spelling is behaviorally equivalent to duramind's `how many` because duramind's match need not consume the following word.
2. A temporal exclusion checked before positive matching: `how many <day|week|month|year>(s) <ago|before|after>`. These ask for elapsed time, not a count of events.
3. A monetary expression: `how much ... <save|saved|spend|spent|raise|raised|earn|earned|in total|altogether>`. This captures sums and differences whose surface form does not contain the shared count phrases.

Duramind has no exclusions and no secondary monetary expression; `IsAggregationQuestion` is simply the shared expression match.

For the 133 multi-session questions, the entire 12-item difference comes from engram-go's monetary expression. The list/every additions do not add a unique positive in this cohort, and the temporal exclusion removes none because the temporal-duration questions belong to the `temporal-reasoning` cohort. Outside the issue's cohort that exclusion matters: it prevents duramind from treating questions such as “How many weeks ago ...?” as aggregation.

`ExtractAggregationAnchor` and the packages' stop-word sets also differ, but neither affects `IsAggregationQuestion`; they are therefore not causal for the 91/79 split.

## Exact symmetric difference and judgments

Snippets are the first 80 characters of the question exactly as stored. An ellipsis marks truncation after character 80.

| `question_id` | Side | First 80 characters | Judgment and inline rationale |
|---|---|---|---|
| `2318644b` | engram-go only | `How much more did I spend on accommodations per night in Hawaii compared to Toky…` | **Ambiguous.** Requires deriving two per-night amounts and subtracting them across sessions. This is multi-fact composition, but not a population count or sum. |
| `129d1232` | engram-go only | `How much money did I raise in total through all the charity events I participate…` | **Engram-go right.** Explicit total over all charity events; paradigmatic aggregation. |
| `d851d5ba` | engram-go only | `How much money did I raise for charity in total?` | **Engram-go right.** Explicit total across multiple charity amounts. |
| `9aaed6a3` | engram-go only | `How much cashback did I earn at SaveMart last Thursday?` | **Ambiguous.** A derived monetary amount may require purchase amount/rate composition, but the wording could also denote one stored cashback fact; it is not intrinsically exhaustive. |
| `e25c3b8d` | engram-go only | `How much did I save on the designer handbag at TK Maxx?` | **Ambiguous.** Requires subtracting paid price from a comparison price, not summing/counting a population. It still needs multiple facts. |
| `4bc144e2` | engram-go only | `How much did I spend on car wash and parking ticket?` | **Engram-go right.** The conjunction asks for the sum of two expenses. |
| `0100672e` | engram-go only | `How much did I spend on each coffee mug for my coworkers?` | **Engram-go right.** “Each” asks for a per-item allocation derived from total spend and item count. It is genuine aggregation/division even though it is not phrased as a total. |
| `bb7c3b45` | engram-go only | `How much did I save on the Jimmy Choo heels?` | **Ambiguous.** A difference between reference and paid prices; multi-fact arithmetic, but not literal aggregation. |
| `ef9cf60a` | engram-go only | `How much did I spend on gifts for my sister?` | **Engram-go right.** Plural gifts imply summing multiple purchases. |
| `09ba9854` | engram-go only | `How much will I save by taking the train from the airport to my hotel instead of…` | **Ambiguous.** Requires subtracting train cost from taxi cost across sessions; operationally compositional, taxonomically a comparison. |
| `078150f1` | engram-go only | `How much more money did I raise than my initial goal in the charity cycling even…` | **Ambiguous.** Requires subtracting the goal from the amount raised; a two-fact difference rather than a sum/count. |
| `09ba9854_abs` | engram-go only | `How much will I save by taking the bus from the airport to my hotel instead of a…` | **Ambiguous.** Same comparison shape as `09ba9854`; the gold answer says information is insufficient, but answering still requires checking both operands across sessions. |

There are **no duramind-only `question_id`s** in the multi-session cohort.

## Recommendation

Use engram-go as the canonical behavior for the current downstream purpose, with two refinements to the contract and implementation design in a future code change:

1. Define the gate as “requires multi-fact retrieval and deterministic composition,” not merely “aggregation/counting.” That definition matches why exhaustive recall and enumerate-first behavior exist and correctly includes sums, per-item calculations, and differences.
2. Separate recognizable shapes internally (or expose a richer classification): `count_or_list`, `sum_or_total`, `difference_or_savings`, and `temporal_duration`. The first three can map to the same operational gate today, while `temporal_duration` remains excluded. This preserves behavior without pretending that every subtraction is a count.

A merged rule should therefore retain:

- the six shared count phrases;
- engram-go's list/every/all-occasion phrases;
- engram-go's temporal-duration negative guard; and
- engram-go's monetary verbs, ideally labeled as a monetary-composition rule rather than folded opaquely into “aggregation.”

One precision improvement is warranted before broad reuse: qualify broad monetary verbs with evidence of multiple operands or multi-item scope when the caller does not already know the question is multi-session. `How much did I spend on a designer handbag?` is a single-fact lookup in the full dataset and is currently an engram-go positive. In the issue's multi-session-only call path, dataset type supplies the missing scope signal; a general-purpose canonical predicate should accept that scope explicitly or use stricter wording cues (`and`, plural object, `each`, `total`, `all`, `more ... than`, `instead of`).

Duramind's smaller predicate is not a better canonical choice: it has no unique correct classifications in the disputed cohort, misses five clear monetary aggregations, misses seven operationally relevant multi-fact calculations, and lacks the temporal false-positive guard. The best canonical result is therefore **engram-go's coverage with an explicit multi-fact-composition contract and narrower context-aware monetary matching**, not a reversion to duramind's six-phrase rule.
