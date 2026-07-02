# LedgerProbe Red-Team Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix six red-team-identified harness artifacts in `internal/ledgerprobe` so that the shallow-extractor exact-match number measures real recall, not measurement noise.

**Architecture:** All changes are test-only or package-internal — no production code is touched. The six fixes are: (1) FailureClass taxonomy on ItemResult + Report; (2) three object-linking arms (lexical_all, lexical_any, no_object_filter) scored independently; (3) DeriveFrame temporal-arithmetic exclusion broadened; (4) ShallowExtractor sentence-split decimal guard + bare-activity object attachment + money-role polarity filter + number-word literals; (5) ParseGoldAnswer multi-candidate support; (6) probe.go denominator / FailingItems exclusions corrected. Each fix builds on the previous, ending with a live probe run.

**Tech Stack:** Go 1.22+, stdlib `regexp`/`strings`/`strconv`/`sort`, no external deps. TDD throughout (`go test ./internal/ledgerprobe/ -race -count=1`).

## Global Constraints

- Test-only / package-internal changes — NO production wiring, no MCP server changes.
- `go vet ./internal/ledgerprobe/` must pass after every task.
- `go test ./internal/ledgerprobe/ -race -count=1` must pass after every task.
- Do NOT `git push`. Do NOT `git commit`. Leave changes unstaged for the lead to review.
- Coverage on new exported functions: ≥ one table test per function.
- Before proposing or selecting any implementation approach, invoke the advisory-gate skill if 2+ approaches exist with meaningfully different consequences (ADV.1-5 triggers).
- When dispatching any subagent, select the lowest model tier and effort level sufficient for the task; include this sentence verbatim in any brief you give to an agent that can itself spawn agents.

---

### Task 1: FailureClass taxonomy — ItemResult + Report enrichment

**Files:**
- Modify: `internal/ledgerprobe/probe.go`
- Modify: `internal/ledgerprobe/ledgerprobe_test.go` (add TestItemResultFailureClass)

**Interfaces:**
- Produces: `ItemResult.FrameDetected bool`, `ItemResult.ExtractedEvents int`, `ItemResult.KeptEvents int`, `ItemResult.FailureClass string` (values: `correct`, `frame_false_positive`, `no_events_extracted`, `events_extracted_but_filter_zero`, `sum_scope_overcount`, `gold_parse_ambiguous`, `exact_wrong_after_kept_events`)
- Produces: `Report.FailureClassDist map[string]int` populated by `RunGoldOracleArm`
- Produces: `Report.String()` now renders the FailureClassDist table

- [ ] **Step 1: Write the failing test**

Add to `internal/ledgerprobe/ledgerprobe_test.go`:

```go
func TestItemResultFailureClass(t *testing.T) {
    cases := []struct {
        name     string
        item     ProbeItem
        wantFC   string
        wantFD   bool
        wantKept int
    }{
        {
            name: "correct",
            item: ProbeItem{
                QuestionID: "q1", Question: "How many model kits have I bought in total?", GoldRaw: "5",
                GoldSessions: map[string]string{
                    "s1": "I bought 2 model kits.",
                    "s2": "Picked up 3 model kits.",
                },
            },
            wantFC: "correct", wantFD: true, wantKept: 2,
        },
        {
            name: "frame_false_positive — temporal question slips through",
            // DeriveFrame returns ok=false for temporal; FailureClass = frame_false_positive
            item: ProbeItem{
                QuestionID: "q2", Question: "What is my favorite color?", GoldRaw: "3",
                GoldSessions: map[string]string{"s1": "I like blue."},
            },
            wantFC: "frame_false_positive", wantFD: false, wantKept: 0,
        },
        {
            name: "no_events_extracted",
            item: ProbeItem{
                QuestionID: "q3", Question: "How many trips have I taken?", GoldRaw: "4",
                GoldSessions: map[string]string{"s1": "The weather was nice."},
            },
            wantFC: "no_events_extracted", wantFD: true, wantKept: 0,
        },
        {
            name: "events_extracted_but_filter_zero",
            item: ProbeItem{
                QuestionID: "q4", Question: "How many Tamiya Spitfires have I built?", GoldRaw: "2",
                // "Tamiya Spitfire" won't token-match "kit" but events exist with object "kit"
                GoldSessions: map[string]string{"s1": "I finished 2 kits yesterday."},
            },
            wantFC: "events_extracted_but_filter_zero", wantFD: true, wantKept: 0,
        },
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            rep := RunGoldOracleArm([]ProbeItem{c.item}, ShallowExtractor{})
            if len(rep.Items) == 0 {
                t.Fatal("no items in report")
            }
            it := rep.Items[0]
            if it.FailureClass != c.wantFC {
                t.Errorf("FailureClass=%q want %q", it.FailureClass, c.wantFC)
            }
            if it.FrameDetected != c.wantFD {
                t.Errorf("FrameDetected=%v want %v", it.FrameDetected, c.wantFD)
            }
            if it.KeptEvents != c.wantKept {
                t.Errorf("KeptEvents=%d want %d", it.KeptEvents, c.wantKept)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/ledgerprobe/ -run TestItemResultFailureClass -v -count=1 2>&1 | tail -20
```

Expected: compile error — fields `FailureClass`, `FrameDetected`, `KeptEvents` undefined.

- [ ] **Step 3: Add fields to ItemResult**

In `internal/ledgerprobe/probe.go`, replace the `ItemResult` struct:

```go
// ItemResult records one item's outcome for the report + failure attribution.
type ItemResult struct {
	QuestionID    string
	Computed      float64
	Gold          float64
	GoldRaw       string
	ExactMatch    bool
	IsAbstention  bool
	NumEvents     int
	Measure       Measure
	// Enriched diagnostics (red-team fix)
	FrameDetected bool
	ExtractedEvents int
	KeptEvents    int
	// FailureClass is one of: correct | frame_false_positive | no_events_extracted |
	// events_extracted_but_filter_zero | sum_scope_overcount | gold_parse_ambiguous |
	// exact_wrong_after_kept_events
	FailureClass  string
}
```

- [ ] **Step 4: Add FailureClassDist to Report**

In `internal/ledgerprobe/probe.go`, replace the `Report` struct:

```go
// Report is the go/no-go signal. ExactMatchRate over countable items is THE number;
// AbstentionItems are excluded from the rate (they need an abstention path, not counting).
type Report struct {
	Extractor        string
	Items            []ItemResult
	Countable        int // numeric-gold items the harness attempted
	Correct          int
	Abstentions      int
	Unparseable      int // gold answer not numeric and not abstention
	ExactMatchRate   float64
	FailureClassDist map[string]int // distribution over FailureClass values
}
```

- [ ] **Step 5: Populate FailureClass in RunGoldOracleArm**

Replace `RunGoldOracleArm` body in `internal/ledgerprobe/probe.go`:

```go
// RunGoldOracleArm executes the full pipeline per item: extract events from gold
// sessions -> derive frame from the question -> deterministic aggregate -> compare
// to gold. No generator is involved.
func RunGoldOracleArm(items []ProbeItem, ex EventExtractor) Report {
	rep := Report{Extractor: ex.Name(), FailureClassDist: map[string]int{}}
	for _, it := range items {
		gold := ParseGoldAnswer(it.GoldRaw)
		res := ItemResult{QuestionID: it.QuestionID, GoldRaw: it.GoldRaw, Gold: gold.Value}

		if gold.IsAbstention {
			res.IsAbstention = true
			res.FailureClass = "abstention"
			rep.Abstentions++
			rep.Items = append(rep.Items, res)
			continue
		}
		if !gold.IsNumeric {
			res.FailureClass = "gold_parse_ambiguous"
			rep.Unparseable++
			rep.Items = append(rep.Items, res)
			continue
		}

		frame, ok := DeriveFrame(it.Question)
		if !ok {
			// frame not detected — this item is NOT a countable aggregation question;
			// exclude from the countable denominator to avoid inflation.
			res.FrameDetected = false
			res.FailureClass = "frame_false_positive"
			rep.Items = append(rep.Items, res)
			continue
		}
		res.FrameDetected = true
		res.Measure = frame.Measure

		var events []EventAtom
		for sid, text := range it.GoldSessions {
			events = append(events, ex.Extract(sid, text)...)
		}
		res.ExtractedEvents = len(events)
		res.NumEvents = len(events)
		total, kept := Aggregate(events, frame)
		res.KeptEvents = len(kept)
		res.Computed = total
		res.ExactMatch = numbersEqual(total, gold.Value)

		// assign failure class
		switch {
		case res.ExactMatch:
			res.FailureClass = "correct"
		case res.ExtractedEvents == 0:
			res.FailureClass = "no_events_extracted"
		case res.KeptEvents == 0:
			res.FailureClass = "events_extracted_but_filter_zero"
		default:
			res.FailureClass = "exact_wrong_after_kept_events"
		}

		rep.Countable++
		rep.FailureClassDist[res.FailureClass]++
		if res.ExactMatch {
			rep.Correct++
		}
		rep.Items = append(rep.Items, res)
	}
	if rep.Countable > 0 {
		rep.ExactMatchRate = float64(rep.Correct) / float64(rep.Countable)
	}
	return rep
}
```

- [ ] **Step 6: Extend Report.String() to render FailureClassDist**

Replace `Report.String()` in `internal/ledgerprobe/probe.go`:

```go
// String renders the go/no-go summary against the pre-registered decision gates.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Oracle-Ledger Probe — extractor=%s\n", r.Extractor)
	fmt.Fprintf(&b, "  countable items:   %d\n", r.Countable)
	fmt.Fprintf(&b, "  exact-match:       %d/%d = %.1f%%\n", r.Correct, r.Countable, r.ExactMatchRate*100)
	fmt.Fprintf(&b, "  abstentions:       %d (excluded from rate)\n", r.Abstentions)
	fmt.Fprintf(&b, "  unparseable gold:  %d\n", r.Unparseable)
	fmt.Fprintf(&b, "  GATE: >=85%% build · 60-85%% ledger+rescue · <60%% abandon count-via-extraction\n")
	if len(r.FailureClassDist) > 0 {
		fmt.Fprintf(&b, "  FailureClass distribution:\n")
		keys := make([]string, 0, len(r.FailureClassDist))
		for k := range r.FailureClassDist {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "    %-45s %d\n", k+":", r.FailureClassDist[k])
		}
	}
	return b.String()
}
```

- [ ] **Step 7: Update FailingItems to exclude frame_false_positive and gold_parse_ambiguous**

Replace `FailingItems` in `internal/ledgerprobe/probe.go`:

```go
// FailingItems returns mismatched countable items sorted by id for inspection.
// Excludes abstentions, unparseable gold (gold_parse_ambiguous), and
// frame_false_positive items — those are not real misses in the extractor.
func (r Report) FailingItems() []ItemResult {
	var out []ItemResult
	for _, it := range r.Items {
		if it.FailureClass == "correct" || it.FailureClass == "abstention" ||
			it.FailureClass == "gold_parse_ambiguous" || it.FailureClass == "frame_false_positive" {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QuestionID < out[j].QuestionID })
	return out
}
```

- [ ] **Step 8: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1 | tail -30
```

Expected: all tests PASS including `TestItemResultFailureClass` and the existing `TestRunGoldOracleArm_EndToEnd`.

---

### Task 2: Three object-linking arms — ArmReport + multi-arm RunGoldOracleArms

**Files:**
- Modify: `internal/ledgerprobe/aggregate.go` (add `AggregateNoFilter`, `AggregateAnyToken`)
- Modify: `internal/ledgerprobe/probe.go` (add `ArmResult`, `MultiArmReport`, `RunGoldOracleArms`)
- Modify: `internal/ledgerprobe/ledgerprobe_test.go` (add `TestMultiArmReport`)
- Modify: `internal/ledgerprobe/probe_runner_test.go` (update `TestOracleLedgerProbe` to call `RunGoldOracleArms`)

**Interfaces:**
- Produces: `AggregateNoFilter(events []EventAtom, f AggregationFrame) (float64, []EventAtom)` — counts/sums ALL extracted events of the right measure, ignoring ObjectTokens (upper bound)
- Produces: `AggregateAnyToken(events []EventAtom, f AggregationFrame) (float64, []EventAtom)` — keeps events where object contains ANY token (OR semantics)
- Produces: `ArmResult{ArmName string, Correct int, Countable int, ExactMatchRate float64}`
- Produces: `MultiArmReport{Arms []ArmResult, ItemArms []ItemArmResult}` where `ItemArmResult` has arm-level computed/kept per item
- Produces: `RunGoldOracleArms(items []ProbeItem, ex EventExtractor) MultiArmReport`

- [ ] **Step 1: Write the failing test**

Add to `internal/ledgerprobe/ledgerprobe_test.go`:

```go
func TestMultiArmReport(t *testing.T) {
    items := []ProbeItem{
        {
            QuestionID: "q-kits", Question: "How many model kits have I bought in total?", GoldRaw: "5",
            GoldSessions: map[string]string{
                // token "kit" present in object "Tamiya kit" — any-token arm should match
                "s1": "I bought a Tamiya kit.",
                "s2": "Picked up 4 Tamiya kits.",
            },
        },
    }
    mr := RunGoldOracleArms(items, ShallowExtractor{})
    // Three arms must be present
    armNames := map[string]bool{}
    for _, a := range mr.Arms {
        armNames[a.ArmName] = true
    }
    for _, want := range []string{"lexical_all_tokens", "lexical_any_token", "no_object_filter"} {
        if !armNames[want] {
            t.Errorf("missing arm %q; got arms: %v", want, mr.Arms)
        }
    }
    // no_object_filter must be >= lexical_all_tokens (upper bound property)
    var allRate, nofRate float64
    for _, a := range mr.Arms {
        switch a.ArmName {
        case "lexical_all_tokens":
            allRate = a.ExactMatchRate
        case "no_object_filter":
            nofRate = a.ExactMatchRate
        }
    }
    if nofRate < allRate-0.001 {
        t.Errorf("no_object_filter rate=%.3f < lexical_all_tokens rate=%.3f — upper-bound invariant violated", nofRate, allRate)
    }
    // ItemArms must have 3 entries per item
    if len(mr.ItemArms) != 3 {
        t.Errorf("ItemArms len=%d want 3 (one per arm)", len(mr.ItemArms))
    }
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/ledgerprobe/ -run TestMultiArmReport -count=1 2>&1 | head -10
```

Expected: compile error — `RunGoldOracleArms`, `MultiArmReport`, `ItemArmResult` undefined.

- [ ] **Step 3: Add AggregateNoFilter and AggregateAnyToken to aggregate.go**

Append to `internal/ledgerprobe/aggregate.go`:

```go
// AggregateNoFilter counts/sums ALL extracted events of the correct measure type,
// ignoring ObjectTokens entirely. This is the UPPER BOUND: "if object linking were
// perfect, can extraction+arithmetic produce the gold count?"
func AggregateNoFilter(events []EventAtom, f AggregationFrame) (float64, []EventAtom) {
	var matched []EventAtom
	switch f.Measure {
	case MeasureSumMoney:
		for _, e := range events {
			if e.EventType == "expense" {
				matched = append(matched, e)
			}
		}
	case MeasureSumQuantity:
		for _, e := range events {
			if f.Unit == "" || e.Unit == f.Unit {
				matched = append(matched, e)
			}
		}
	default:
		matched = append(matched, events...)
	}
	kept := dedup(matched)
	var total float64
	switch f.Measure {
	case MeasureSumMoney:
		for _, e := range kept {
			total += float64(sign(e.Polarity)) * e.Money
		}
	case MeasureSumQuantity:
		for _, e := range kept {
			total += float64(sign(e.Polarity)) * nonZero(e.Quantity)
		}
	default:
		for _, e := range kept {
			total += float64(sign(e.Polarity)) * nonZero(e.Quantity)
		}
	}
	return total, kept
}

// AggregateAnyToken keeps events whose object contains ANY of the frame's object
// tokens (OR semantics). More permissive than Aggregate (AND), less permissive than
// AggregateNoFilter.
func AggregateAnyToken(events []EventAtom, f AggregationFrame) (float64, []EventAtom) {
	if f.Measure != MeasureCount || len(f.ObjectTokens) == 0 {
		// for sum arms the filter logic is the same as no-filter — delegate
		return AggregateNoFilter(events, f)
	}
	var matched []EventAtom
	for _, e := range events {
		obj := strings.ToLower(e.Object + " " + e.EventType)
		for _, t := range f.ObjectTokens {
			if strings.Contains(obj, t) {
				matched = append(matched, e)
				break
			}
		}
	}
	kept := dedup(matched)
	var total float64
	for _, e := range kept {
		total += float64(sign(e.Polarity)) * nonZero(e.Quantity)
	}
	return total, kept
}
```

- [ ] **Step 4: Add ArmResult, ItemArmResult, MultiArmReport to probe.go**

Add after the `Report` struct in `internal/ledgerprobe/probe.go`:

```go
// ArmResult records aggregate exact-match for one object-linking arm.
type ArmResult struct {
	ArmName        string
	Correct        int
	Countable      int
	ExactMatchRate float64
}

// ItemArmResult records per-item, per-arm computed value + kept events.
type ItemArmResult struct {
	QuestionID string
	ArmName    string
	Computed   float64
	KeptEvents int
	ExactMatch bool
}

// MultiArmReport scores all three object-linking arms in one pass.
// Arms[0]=lexical_all_tokens (current AND), Arms[1]=lexical_any_token (OR),
// Arms[2]=no_object_filter (UPPER BOUND).
type MultiArmReport struct {
	Extractor string
	Arms      []ArmResult
	ItemArms  []ItemArmResult
	// SingleArm is the primary ItemResult-level report (lexical_all_tokens arm).
	SingleArm Report
}

// String renders a compact arm-comparison table.
func (m MultiArmReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Multi-Arm Oracle-Ledger Probe — extractor=%s\n", m.Extractor)
	fmt.Fprintf(&b, "  %-30s %8s %8s %10s\n", "Arm", "Correct", "Countable", "ExactMatch%")
	for _, a := range m.Arms {
		fmt.Fprintf(&b, "  %-30s %8d %8d %9.1f%%\n", a.ArmName, a.Correct, a.Countable, a.ExactMatchRate*100)
	}
	fmt.Fprintf(&b, "\n%s", m.SingleArm.String())
	return b.String()
}
```

- [ ] **Step 5: Add RunGoldOracleArms to probe.go**

Add after `RunGoldOracleArm` in `internal/ledgerprobe/probe.go`:

```go
// RunGoldOracleArms runs all three object-linking arms in one pass and returns a
// MultiArmReport. The SingleArm field carries the full ItemResult diagnostics for
// the lexical_all_tokens arm (same as RunGoldOracleArm).
func RunGoldOracleArms(items []ProbeItem, ex EventExtractor) MultiArmReport {
	armNames := []string{"lexical_all_tokens", "lexical_any_token", "no_object_filter"}
	arms := make([]ArmResult, len(armNames))
	for i, n := range armNames {
		arms[i].ArmName = n
	}

	primary := RunGoldOracleArm(items, ex) // lexical_all_tokens (existing logic)
	// copy arm-0 totals from primary
	arms[0].Correct = primary.Correct
	arms[0].Countable = primary.Countable
	if primary.Countable > 0 {
		arms[0].ExactMatchRate = float64(primary.Correct) / float64(primary.Countable)
	}

	var itemArms []ItemArmResult
	// Arm 0: lexical_all_tokens — derive from primary.Items
	for _, it := range primary.Items {
		if it.FrameDetected && it.Gold != 0 || it.ExactMatch {
			itemArms = append(itemArms, ItemArmResult{
				QuestionID: it.QuestionID, ArmName: "lexical_all_tokens",
				Computed: it.Computed, KeptEvents: it.KeptEvents, ExactMatch: it.ExactMatch,
			})
		}
	}

	// Arms 1+2: re-run aggregation with alternate filters for countable items
	arm1 := ArmResult{ArmName: "lexical_any_token"}
	arm2 := ArmResult{ArmName: "no_object_filter"}

	for _, it := range items {
		gold := ParseGoldAnswer(it.GoldRaw)
		if gold.IsAbstention || !gold.IsNumeric {
			continue
		}
		frame, ok := DeriveFrame(it.Question)
		if !ok {
			continue
		}
		var events []EventAtom
		for sid, text := range it.GoldSessions {
			events = append(events, ex.Extract(sid, text)...)
		}

		// arm 1: any-token
		t1, k1 := AggregateAnyToken(events, frame)
		match1 := numbersEqual(t1, gold.Value)
		arm1.Countable++
		if match1 {
			arm1.Correct++
		}
		itemArms = append(itemArms, ItemArmResult{
			QuestionID: it.QuestionID, ArmName: "lexical_any_token",
			Computed: t1, KeptEvents: len(k1), ExactMatch: match1,
		})

		// arm 2: no filter
		t2, k2 := AggregateNoFilter(events, frame)
		match2 := numbersEqual(t2, gold.Value)
		arm2.Countable++
		if match2 {
			arm2.Correct++
		}
		itemArms = append(itemArms, ItemArmResult{
			QuestionID: it.QuestionID, ArmName: "no_object_filter",
			Computed: t2, KeptEvents: len(k2), ExactMatch: match2,
		})
	}
	if arm1.Countable > 0 {
		arm1.ExactMatchRate = float64(arm1.Correct) / float64(arm1.Countable)
	}
	if arm2.Countable > 0 {
		arm2.ExactMatchRate = float64(arm2.Correct) / float64(arm2.Countable)
	}
	arms[1] = arm1
	arms[2] = arm2

	return MultiArmReport{
		Extractor: ex.Name(),
		Arms:      arms,
		ItemArms:  itemArms,
		SingleArm: primary,
	}
}
```

- [ ] **Step 6: Update TestOracleLedgerProbe in probe_runner_test.go to use RunGoldOracleArms**

Replace the body of `TestOracleLedgerProbe` in `internal/ledgerprobe/probe_runner_test.go`:

```go
func TestOracleLedgerProbe(t *testing.T) {
	path := os.Getenv("LEDGERPROBE_DATA")
	if path == "" {
		t.Skip("set LEDGERPROBE_DATA to the LongMemEval json to run the live probe")
	}
	items, err := loadProbeItems(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	mr := RunGoldOracleArms(items, ShallowExtractor{})
	t.Logf("\n%s", mr.String())
	for _, f := range mr.SingleArm.FailingItems() {
		t.Logf("  MISS %s: fc=%s computed=%.0f gold=%.0f (%q) extracted=%d kept=%d measure=%d",
			f.QuestionID, f.FailureClass, f.Computed, f.Gold, f.GoldRaw,
			f.ExtractedEvents, f.KeptEvents, f.Measure)
	}
	t.Logf("baseline shallow-extractor gold-oracle exact-match = %.1f%% on %d countable items",
		mr.SingleArm.ExactMatchRate*100, mr.SingleArm.Countable)
}
```

- [ ] **Step 7: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1 | tail -30
```

Expected: all tests PASS including `TestMultiArmReport`.

---

### Task 3: DeriveFrame — broaden temporal-arithmetic exclusion

**Files:**
- Modify: `internal/ledgerprobe/frame.go`
- Modify: `internal/ledgerprobe/ledgerprobe_test.go` (extend TestDeriveFrame with new temporal cases)

**Interfaces:**
- No changes to `DeriveFrame` signature.
- Produces: `DeriveFrameStats` struct and `DeriveFrame` extended to also return it — but keeping the existing two-return-value signature, stats tracked separately via a new `DeriveFrameStats` accumulator type and `DeriveFrameWithStats(question string) (AggregationFrame, bool, DeriveFrameStats)`.

Note: the probe callers use `DeriveFrame` (two returns). Add `DeriveFrameWithStats` as a new function and have `DeriveFrame` delegate to it to avoid breaking existing callers.

- [ ] **Step 1: Write failing tests**

In `internal/ledgerprobe/ledgerprobe_test.go`, extend `TestDeriveFrame` cases slice with:

```go
// Add these cases to the existing cases slice:
{"How many days since I last ran?", false, MeasureNone, nil},           // "days since" temporal
{"How many weeks elapsed since my last visit?", false, MeasureNone, nil}, // "elapsed since"
{"How many months has it been between my two trips?", false, MeasureNone, nil}, // "between...and"
{"How long had the project been running?", false, MeasureNone, nil},    // "how long had"
{"How many miles did I run from January to March?", false, MeasureNone, nil}, // "from...to" scope
```

Also add a new test:

```go
func TestDeriveFrameWithStats(t *testing.T) {
    cases := []struct {
        q        string
        ok       bool
        rejected bool // true = temporal-rejected
    }{
        {"How many model kits have I bought?", true, false},
        {"How many days since I last ran?", false, true},
        {"How many weeks elapsed since my last visit?", false, true},
        {"How many months between my two trips?", false, true},
        {"How long had the project been running?", false, true},
    }
    for _, c := range cases {
        _, ok, stats := DeriveFrameWithStats(c.q)
        if ok != c.ok {
            t.Errorf("DeriveFrameWithStats(%q) ok=%v want %v", c.q, ok, c.ok)
        }
        if c.rejected && !stats.TemporalRejected {
            t.Errorf("DeriveFrameWithStats(%q) TemporalRejected=false want true", c.q)
        }
    }
}
```

- [ ] **Step 2: Run to confirm failing**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/ledgerprobe/ -run TestDeriveFrame -count=1 2>&1 | head -20
```

Expected: TestDeriveFrame failures on the new temporal cases; `DeriveFrameWithStats` compile error.

- [ ] **Step 3: Broaden frameTemporalExclude regex and add DeriveFrameWithStats**

Replace the `frameTemporalExclude` declaration and the `DeriveFrame` function block in `internal/ledgerprobe/frame.go`:

```go
// relative-time and date-range arithmetic are temporal reasoning, not aggregation.
// Excluded so that precision-biased DeriveFrame does not admit them.
frameTemporalExclude = regexp.MustCompile(
    `(?i)` +
        `\b(passed since|elapsed since)\b` +
        `|\bdays? since\b` +
        `|\bweeks? since\b` +
        `|\bmonths? since\b` +
        `|\byears? since\b` +
        `|\bhours? since\b` +
        `|\bminutes? since\b` +
        `|\bbetween\b.{1,60}\band\b` +
        `|\bfrom\b.{1,40}\bto\b` +
        `|\bhow long\b`,
)
```

Add after `DeriveFrame`:

```go
// DeriveFrameStats records why a question was accepted or rejected.
type DeriveFrameStats struct {
	TemporalRejected bool
	NotAggregation   bool
}

// DeriveFrameWithStats is like DeriveFrame but also returns diagnostic stats.
func DeriveFrameWithStats(question string) (AggregationFrame, bool, DeriveFrameStats) {
	q := strings.ToLower(question)
	var stats DeriveFrameStats
	if frameTemporalExclude.MatchString(q) {
		stats.TemporalRejected = true
		return AggregationFrame{}, false, stats
	}
	money := frameMoneyRe.MatchString(q)
	count := frameCountRe.MatchString(q)
	if !money && !count {
		stats.NotAggregation = true
		return AggregationFrame{}, false, stats
	}
	f := AggregationFrame{ObjectTokens: objectTokens(q)}
	switch {
	case money:
		f.Measure = MeasureSumMoney
		f.Unit = "dollar"
	default:
		if u := unitOf(q); u != "" && u != "times" {
			f.Measure = MeasureSumQuantity
			f.Unit = u
		} else {
			f.Measure = MeasureCount
		}
	}
	return f, true, stats
}

// DeriveFrame delegates to DeriveFrameWithStats — existing callers are unaffected.
func DeriveFrame(question string) (AggregationFrame, bool) {
	f, ok, _ := DeriveFrameWithStats(question)
	return f, ok
}
```

Remove the original `DeriveFrame` implementation (it is now the delegate body — replace it entirely with the delegate call above).

- [ ] **Step 4: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1 | tail -20
```

Expected: all tests PASS.

---

### Task 4: ShallowExtractor — four targeted fixes

**Files:**
- Modify: `internal/ledgerprobe/extract_shallow.go`
- Modify: `internal/ledgerprobe/ledgerprobe_test.go` (extend TestShallowExtractor + add targeted sub-tests)

The four fixes:
- 4a. `splitSentences` must NOT split on a period between digits (`$12.50`, `3.5 hours`).
- 4b. Bare activity events attach an Object phrase extracted from the verb's NP (`"bought a blazer"` → Object `"blazer"`).
- 4c. Number words `a/an/one..ten` map to their numeric values.
- 4d. Align `normUnit` and `frame.go`'s `unitOf` (both return `"km"` for `"kilometer"`, `"hour"` for `"minute"`, etc.).

Note: the money-role (spent/saved/earned polarity) fix is deferred to Task 5 (it requires role tagging that depends on Task 4's verb-object work).

**Interfaces:**
- No new exported functions; all changes are within `ShallowExtractor.Extract`, `splitSentences`, `normUnit`.
- Produces: `EventAtom.Object` populated for bare-activity events.

- [ ] **Step 1: Write failing tests for 4a, 4b, 4c, 4d**

Replace `TestShallowExtractor` in `internal/ledgerprobe/ledgerprobe_test.go`:

```go
func TestShallowExtractor(t *testing.T) {
	ex := ShallowExtractor{}
	ev := ex.Extract("s1", "On 2024-03-02 I bought 2 Gundam model kits. Later I spent $440 on tools. I ran 5 km that day.")
	var hasKit, hasMoney, hasKm bool
	for _, e := range ev {
		switch {
		case e.EventType == "count" && e.Quantity == 2:
			hasKit = true
		case e.EventType == "expense" && e.Money == 440:
			hasMoney = true
		case e.Unit == "km" && e.Quantity == 5:
			hasKm = true
		}
	}
	if !hasKit || !hasMoney || !hasKm {
		t.Errorf("shallow extract missed: kit=%v money=%v km=%v (got %d events)", hasKit, hasMoney, hasKm, len(ev))
	}
}

func TestShallowExtractor_DecimalSentenceSplit(t *testing.T) {
	// "$12.50" must not be split mid-token; should yield one expense event with Money=12.50
	ex := ShallowExtractor{}
	ev := ex.Extract("s1", "I paid $12.50 for a coffee and ran 3.5 km.")
	var hasMoney, hasKm bool
	for _, e := range ev {
		if e.EventType == "expense" && e.Money > 12 && e.Money < 13 {
			hasMoney = true
		}
		if e.Unit == "km" && e.Quantity > 3 && e.Quantity < 4 {
			hasKm = true
		}
	}
	if !hasMoney {
		t.Errorf("decimal money not extracted: events=%+v", ev)
	}
	if !hasKm {
		t.Errorf("decimal km not extracted: events=%+v", ev)
	}
}

func TestShallowExtractor_BareActivityObject(t *testing.T) {
	// "bought a blazer" → bare activity event should have Object="blazer"
	ex := ShallowExtractor{}
	ev := ex.Extract("s1", "I bought a blazer today.")
	var found bool
	for _, e := range ev {
		if e.EventType == "activity" && strings.Contains(strings.ToLower(e.Object), "blazer") {
			found = true
		}
	}
	if !found {
		t.Errorf("bare activity object not attached; events=%+v", ev)
	}
}

func TestShallowExtractor_NumberWords(t *testing.T) {
	// "a model kit" → quantity 1; "three model kits" → quantity 3
	ex := ShallowExtractor{}
	ev := ex.Extract("s1", "I picked up a model kit and three pairs of shoes.")
	var hasOne, hasThree bool
	for _, e := range ev {
		if e.Quantity == 1 && strings.Contains(strings.ToLower(e.Object), "kit") {
			hasOne = true
		}
		if e.Quantity == 3 && strings.Contains(strings.ToLower(e.Object), "shoe") {
			hasThree = true
		}
	}
	if !hasOne {
		t.Errorf("number word 'a' not parsed as 1; events=%+v", ev)
	}
	if !hasThree {
		t.Errorf("number word 'three' not parsed as 3; events=%+v", ev)
	}
}

func TestNormUnitConsistency(t *testing.T) {
	// normUnit and unitOf must agree on canonical values
	pairs := []struct{ raw, want string }{
		{"hours", "hour"}, {"hrs", "hour"}, {"hour", "hour"},
		{"km", "km"}, {"kilometers", "km"}, {"kilometres", "km"}, {"miles", "km"},
		{"minutes", "hour"}, // normalized to hour (coarse)
		{"pages", "page"},
	}
	for _, p := range pairs {
		got := normUnit(p.raw)
		if got != p.want {
			t.Errorf("normUnit(%q)=%q want %q", p.raw, got, p.want)
		}
	}
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/ledgerprobe/ -run 'TestShallowExtractor|TestNormUnit' -count=1 -v 2>&1 | head -40
```

Expected: `TestShallowExtractor_DecimalSentenceSplit`, `TestShallowExtractor_BareActivityObject`, `TestShallowExtractor_NumberWords`, `TestNormUnitConsistency` fail.

- [ ] **Step 3: Fix splitSentences — do not split on period between digits**

Replace `splitSentences` in `internal/ledgerprobe/extract_shallow.go`:

```go
// splitSentences splits text into sentences. A period that is flanked by a digit
// on either side is a decimal and must NOT be treated as a sentence boundary
// ($12.50, 3.5 hours). The regex negative-lookahead approach is cleaner than
// a post-hoc rejoin step.
var seSentSplit = regexp.MustCompile(`[!?\n]+|\.+(?:[^0-9]|$)`)

func splitSentences(text string) []string {
	// Protect decimal points: temporarily replace digit.digit with a placeholder,
	// split, then restore.
	protected := regexp.MustCompile(`(\d)\.(\d)`).ReplaceAllString(text, `$1‹DOT›$2`)
	raw := seSentSplit.Split(protected, -1)
	var out []string
	for _, p := range raw {
		p = strings.ReplaceAll(p, "‹DOT›", ".")
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 4: Fix normUnit alignment**

Replace `normUnit` in `internal/ledgerprobe/extract_shallow.go`:

```go
func normUnit(u string) string {
	u = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(u, "s"), "r"))
	// re-trim double-strip edge cases
	u = strings.ToLower(u)
	switch u {
	case "hr", "hou", "hour":
		return "hour"
	case "minute", "minut":
		return "hour" // coarse normalization matches frame.go unitOf
	case "km", "kilometer", "kilomete", "kilometre", "kilomet", "mile", "mil":
		return "km"
	case "page", "pag":
		return "page"
	case "day", "da":
		return "day"
	}
	return u
}
```

Note: The TrimSuffix approach for "s"/"r" is fragile. Use a cleaner approach:

```go
func normUnit(u string) string {
	u = strings.ToLower(u)
	// strip plural s but not from "km" or short units
	if len(u) > 3 && strings.HasSuffix(u, "s") {
		u = u[:len(u)-1]
	}
	switch u {
	case "hr", "hour":
		return "hour"
	case "minute":
		return "hour" // coarse normalization, matches frame.go unitOf
	case "km", "kilometer", "kilometre", "mile":
		return "km"
	case "page":
		return "page"
	case "day":
		return "day"
	}
	return u
}
```

- [ ] **Step 5: Add number-word map and extend seQtyObj**

Add after the `var (` block declarations in `internal/ledgerprobe/extract_shallow.go`:

```go
// seNumberWord matches English number words in front of an object noun phrase.
var seNumberWord = regexp.MustCompile(`(?i)\b(a|an|one|two|three|four|five|six|seven|eight|nine|ten)\s+([a-z][a-z\- ]{1,40})`)

var numberWords = map[string]float64{
	"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4,
	"five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}
```

In the `Extract` method, after the `seQtyObj` loop, add:

```go
// number-word quantity + object ("a model kit", "three pairs of shoes")
for _, m := range seNumberWord.FindAllStringSubmatch(sent, -1) {
	if seUnitQty.MatchString(m[0]) {
		continue
	}
	q := numberWords[strings.ToLower(m[1])]
	obj := strings.TrimSpace(m[2])
	// avoid duplicating what seQtyObj already captured
	alreadyCaptured := false
	for _, e := range events {
		if e.Object == obj && e.SessionID == sessionID {
			alreadyCaptured = true
			break
		}
	}
	if !alreadyCaptured {
		events = append(events, EventAtom{
			SessionID: sessionID, EventType: "count", Object: obj, Quantity: q,
			Unit: "item", Polarity: pol, ObservedAt: date, SourceSpan: trimSpan(sent), Confidence: 0.35,
		})
	}
}
```

- [ ] **Step 6: Attach Object to bare activity events**

Add a bare-activity verb → following-noun-phrase regex:

```go
// seVerbNP captures the direct object NP following a transitive activity verb.
var seVerbNP = regexp.MustCompile(`(?i)\b(?:bought|purchased|made|finished|read|watched|completed|donated|received|got)\s+(?:a|an|the|one|my)?\s*([a-z][a-z\- ]{1,40})`)
```

Replace the bare-activity block in `Extract`:

```go
// bare activity events — attach object from NP if present
if seActivity.MatchString(sent) && !seQtyObj.MatchString(sent) && !seMoney.MatchString(sent) {
    obj := ""
    if m := seVerbNP.FindStringSubmatch(sent); len(m) > 1 {
        obj = strings.TrimSpace(m[1])
    }
    events = append(events, EventAtom{
        SessionID: sessionID, EventType: "activity", Object: obj, Quantity: 1, Unit: "item",
        Polarity: pol, ObservedAt: date, SourceSpan: trimSpan(sent), Confidence: 0.3,
    })
}
```

- [ ] **Step 7: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1 | tail -30
```

Expected: all tests PASS.

---

### Task 5: ParseGoldAnswer — multi-candidate support

**Files:**
- Modify: `internal/ledgerprobe/goldanswer.go`
- Modify: `internal/ledgerprobe/ledgerprobe_test.go` (extend TestParseGoldAnswer with multi-candidate cases)

**Interfaces:**
- Extends `ParsedGold` with: `Candidates []float64`, `IsAmbiguous bool`
- `ParseGoldAnswer` populates `Candidates` when multiple numeric values appear with "acceptable" or "or" between them; sets `IsAmbiguous=true` when no clear primary.
- `numbersEqual` is replaced by `numbersEqualAny(computed float64, g ParsedGold) bool` that matches if computed equals ANY candidate.

- [ ] **Step 1: Write failing tests**

Add to `TestParseGoldAnswer` in `internal/ledgerprobe/ledgerprobe_test.go`:

```go
// Add these cases to the existing cases slice in TestParseGoldAnswer:
{"14 days. 15 days is also acceptable.", 14, true, false},
// ambiguous: both 14 and 15 are valid; IsAmbiguous=true, Candidates=[14,15]
```

And add a new test:

```go
func TestParseGoldAnswerMultiCandidate(t *testing.T) {
    cases := []struct {
        raw        string
        candidates []float64
        ambiguous  bool
        primary    float64
    }{
        {
            raw:        "14 days. 15 days is also acceptable.",
            candidates: []float64{14, 15},
            ambiguous:  true,
            primary:    14,
        },
        {
            raw:        "3",
            candidates: []float64{3},
            ambiguous:  false,
            primary:    3,
        },
        {
            raw:        "approximately 5 or 6 items",
            candidates: []float64{5, 6},
            ambiguous:  true,
            primary:    5,
        },
    }
    for _, c := range cases {
        g := ParseGoldAnswer(c.raw)
        if g.IsAmbiguous != c.ambiguous {
            t.Errorf("ParseGoldAnswer(%q) IsAmbiguous=%v want %v", c.raw, g.IsAmbiguous, c.ambiguous)
        }
        if len(g.Candidates) != len(c.candidates) {
            t.Errorf("ParseGoldAnswer(%q) Candidates=%v want %v", c.raw, g.Candidates, c.candidates)
        }
        if g.Value != c.primary {
            t.Errorf("ParseGoldAnswer(%q) Value=%v want %v (primary)", c.raw, g.Value, c.primary)
        }
    }
}

func TestNumbersEqualAny(t *testing.T) {
    cases := []struct {
        computed float64
        raw      string
        want     bool
    }{
        {14, "14 days. 15 days is also acceptable.", true},
        {15, "14 days. 15 days is also acceptable.", true},
        {16, "14 days. 15 days is also acceptable.", false},
        {3, "3", true},
        {4, "3", false},
    }
    for _, c := range cases {
        g := ParseGoldAnswer(c.raw)
        got := numbersEqualAny(c.computed, g)
        if got != c.want {
            t.Errorf("numbersEqualAny(%.0f, %q)=%v want %v (candidates=%v)", c.computed, c.raw, got, c.want, g.Candidates)
        }
    }
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/ledgerprobe/ -run 'TestParseGold|TestNumbersEqualAny' -count=1 2>&1 | head -20
```

Expected: compile errors on `IsAmbiguous`, `Candidates`, `numbersEqualAny`.

- [ ] **Step 3: Extend ParsedGold and ParseGoldAnswer**

Replace `goldanswer.go` entirely:

```go
package ledgerprobe

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedGold is a gold answer reduced to a comparable numeric form. Aggregation
// answers in LongMemEval are short ("38", "$3,750", "140 hours", "8 miles"); some
// are abstentions ("the information provided is not enough"), which are not
// countable and are scored separately.
type ParsedGold struct {
	Value        float64
	Candidates   []float64 // all valid numeric values when multiple are acceptable
	IsNumeric    bool
	IsAmbiguous  bool // true when Candidates has 2+ values (exclude from exact-match denominator)
	IsAbstention bool
	Raw          string
}

var (
	goldAbstainRe = regexp.MustCompile(`(?i)\b(not enough|insufficient|cannot be determined|not (?:mentioned|stated|specified|provided)|no information|did not (?:mention|say))\b`)
	goldNumberRe  = regexp.MustCompile(`-?\$?\s?[0-9][0-9,]*(?:\.[0-9]+)?`)
	// goldAmbiguousRe detects "also acceptable", "or N", "either" hints
	goldAmbiguousRe = regexp.MustCompile(`(?i)\b(also acceptable|either|approximately)\b`)
)

// ParseGoldAnswer reduces a gold answer string to a number when possible. When
// multiple numeric candidates are found (e.g. "14 days. 15 days is also acceptable.")
// all are returned in Candidates and IsAmbiguous=true. The FIRST number is Value
// (primary). Ambiguous items should be excluded from the exact-match denominator.
func ParseGoldAnswer(raw string) ParsedGold {
	g := ParsedGold{Raw: raw}
	if goldAbstainRe.MatchString(raw) {
		g.IsAbstention = true
		return g
	}

	ms := goldNumberRe.FindAllString(raw, -1)
	if len(ms) == 0 {
		return g
	}

	var candidates []float64
	for _, m := range ms {
		clean := strings.NewReplacer("$", "", ",", "", " ", "").Replace(m)
		v, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			continue
		}
		candidates = append(candidates, v)
	}
	if len(candidates) == 0 {
		return g
	}

	g.Value = candidates[0]
	g.Candidates = candidates
	g.IsNumeric = true

	// ambiguous when multiple distinct candidates or explicit "also acceptable" hint
	if len(candidates) > 1 || goldAmbiguousRe.MatchString(raw) {
		g.IsAmbiguous = true
	}
	return g
}

// numbersEqualAny returns true if computed matches ANY candidate in g (within tolerance).
// Falls back to single-value comparison when Candidates is empty.
func numbersEqualAny(computed float64, g ParsedGold) bool {
	candidates := g.Candidates
	if len(candidates) == 0 {
		candidates = []float64{g.Value}
	}
	for _, c := range candidates {
		if numbersEqual(computed, c) {
			return true
		}
	}
	return false
}

// numbersEqual compares the computed total to gold with a tiny tolerance so that
// 3 == 3.0 and money rounding does not spuriously fail an otherwise-correct count.
func numbersEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.5
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
```

- [ ] **Step 4: Update RunGoldOracleArm to use numbersEqualAny and exclude ambiguous from denominator**

In `internal/ledgerprobe/probe.go`, in `RunGoldOracleArm`:

Replace:
```go
if !gold.IsNumeric {
    res.FailureClass = "gold_parse_ambiguous"
    rep.Unparseable++
    rep.Items = append(rep.Items, res)
    continue
}
```

With:
```go
if !gold.IsNumeric {
    res.FailureClass = "gold_parse_ambiguous"
    rep.Unparseable++
    rep.Items = append(rep.Items, res)
    continue
}
if gold.IsAmbiguous {
    // multi-candidate gold — exclude from denominator, record separately
    res.FailureClass = "gold_parse_ambiguous"
    rep.Unparseable++
    rep.Items = append(rep.Items, res)
    continue
}
```

Also replace `res.ExactMatch = numbersEqual(total, gold.Value)` with:
```go
res.ExactMatch = numbersEqualAny(total, gold)
```

Similarly update `RunGoldOracleArms` (arms 1 and 2 re-run blocks):

Replace:
```go
match1 := numbersEqual(t1, gold.Value)
```
with:
```go
match1 := numbersEqualAny(t1, gold)
```

Replace:
```go
match2 := numbersEqual(t2, gold.Value)
```
with:
```go
match2 := numbersEqualAny(t2, gold)
```

- [ ] **Step 5: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1 | tail -30
```

Expected: all tests PASS.

---

### Task 6: End-to-end verification — vet + test suite + live probe

**Files:**
- Read-only verification pass; no code changes expected (fix any discovered compile/vet issues in place).

- [ ] **Step 1: Full vet + race test**

```bash
cd /home/psimmons/projects/engram-go && go vet ./internal/ledgerprobe/ && go test ./internal/ledgerprobe/ -race -count=1 -v 2>&1
```

Expected: `ok  github.com/petersimmons1972/engram/internal/ledgerprobe` — all tests PASS.

- [ ] **Step 2: Live probe against LongMemEval (requires LEDGERPROBE_DATA)**

```bash
LEDGERPROBE_DATA=$HOME/projects/engram-go/testdata/longmemeval/longmemeval_m_v9failures_135.json \
  go test ./internal/ledgerprobe/ -run TestOracleLedgerProbe -v -count=1 -timeout 300s 2>&1
```

Expected output structure:
```
Multi-Arm Oracle-Ledger Probe — extractor=shallow-regex
  Arm                            Correct Countable ExactMatch%
  lexical_all_tokens                   N        M       X.X%
  lexical_any_token                    N        M       X.X%
  no_object_filter                     N        M       Y.Y%   ← KEY NUMBER

Oracle-Ledger Probe — extractor=shallow-regex
  countable items:   M
  exact-match:       N/M = X.X%
  FailureClass distribution:
    correct:                                      N
    events_extracted_but_filter_zero:             N   ← expect this to be large (the harness artifact)
    exact_wrong_after_kept_events:                N
    no_events_extracted:                          N
```

- [ ] **Step 3: Record results**

After the live probe runs, the lead should note:
- `no_object_filter` exact-match % = **the upper bound** (what the arithmetic layer can achieve if linking were perfect)
- `lexical_all_tokens` % = current (previously reported 4.7%)
- `events_extracted_but_filter_zero` count = how many items were dominated by the token-AND artifact

---

## Self-Review

### Spec coverage check

| Spec requirement | Task | Status |
|-----------------|------|--------|
| FailureClass taxonomy on ItemResult | Task 1 | ✅ |
| FrameDetected, ExtractedEvents, KeptEvents fields | Task 1 | ✅ |
| FailureClassDist on Report | Task 1 | ✅ |
| Three object-linking arms (lexical_all, lexical_any, no_object_filter) | Task 2 | ✅ |
| no_object_filter is the key deliverable (upper bound) | Task 2 | ✅ |
| DeriveFrame: exclude temporal date-arithmetic patterns | Task 3 | ✅ |
| DeriveFrameWithStats accepted-vs-rejected stats | Task 3 | ✅ |
| splitSentences decimal guard | Task 4 | ✅ |
| Bare activity → Object attachment | Task 4 | ✅ |
| Number words (a/an/one..ten) | Task 4 | ✅ |
| normUnit / unitOf alignment | Task 4 | ✅ |
| ParseGoldAnswer multi-candidate | Task 5 | ✅ |
| Ambiguous gold excluded from denominator | Task 5 | ✅ |
| frame-not-detected does NOT inflate denominator | Task 1 Step 5 | ✅ |
| FailingItems excludes frame_false_positive + gold_parse_ambiguous | Task 1 Step 7 | ✅ |
| go vet + go test -race must pass | Task 6 | ✅ |
| Live probe run with new output | Task 6 | ✅ |
| No git push / no git commit | Global constraints | ✅ |

### Spec gaps

- Money-role polarity by verb (spent/saved/earned → filter by role) — the spec lists this as Fix 4 but it requires knowing which role the question asks for (the frame would need a `MoneyRole` field). This is architecturally significant (ADV.1 trigger) and is NOT covered in this plan. Recommend a follow-on plan after seeing the live probe results — the no_object_filter number will tell us whether arithmetic is even the bottleneck before investing in role tagging.

### Placeholder scan

No TBD, TODO, or "implement later" in any step. All code is shown verbatim.

### Type consistency check

- `ItemResult.FailureClass` string — used in Task 1, referenced in Tasks 2/5. ✅
- `ItemResult.KeptEvents` int — set in Task 1, referenced in MISS log in Task 2. ✅
- `ParsedGold.Candidates []float64` — defined in Task 5, consumed by `numbersEqualAny` in same task, called from `RunGoldOracleArm` and `RunGoldOracleArms`. ✅
- `DeriveFrameWithStats` three-return-value signature — `DeriveFrame` delegates; all existing callers unchanged. ✅
- `AggregateNoFilter` / `AggregateAnyToken` signatures — used verbatim in `RunGoldOracleArms`. ✅
