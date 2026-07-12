package atom_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/types"
)

// ── stub backend ──────────────────────────────────────────────────────────────

type stubBackend struct {
	jobs          []atom.ExtractionJob
	memories      map[string]*types.Memory
	existing      []atom.Atom
	inserted      []atom.Atom
	retired       []string
	retiredAt     map[string]time.Time
	mutations     []string
	retireErr     error
	completedJobs map[string]error
}

func newStubBackend() *stubBackend {
	return &stubBackend{
		memories:      make(map[string]*types.Memory),
		retiredAt:     make(map[string]time.Time),
		completedJobs: make(map[string]error),
	}
}

func (s *stubBackend) ClaimAtomExtractionJobs(_ context.Context, _ string, limit int) ([]atom.ExtractionJob, error) {
	n := len(s.jobs)
	if n > limit {
		n = limit
	}
	claimed := s.jobs[:n]
	s.jobs = s.jobs[n:]
	return claimed, nil
}

func (s *stubBackend) CompleteAtomExtractionJob(_ context.Context, jobID string, err error) error {
	s.completedJobs[jobID] = err
	return nil
}

func (s *stubBackend) GetMemory(_ context.Context, id string) (*types.Memory, error) {
	m, ok := s.memories[id]
	if !ok {
		return nil, nil
	}
	return m, nil
}

func (s *stubBackend) GetActiveAtoms(_ context.Context, _ string, _ string) ([]atom.Atom, error) {
	active := make([]atom.Atom, 0, len(s.existing))
	for _, existing := range s.existing {
		if existing.ValidTo == nil {
			active = append(active, existing)
		}
	}
	return active, nil
}

func (s *stubBackend) InsertAtom(_ context.Context, a *atom.Atom) error {
	for _, existing := range s.existing {
		if existing.ID == a.ID {
			return nil
		}
	}
	s.inserted = append(s.inserted, *a)
	s.existing = append(s.existing, *a)
	s.mutations = append(s.mutations, "insert:"+a.ID)
	return nil
}

func (s *stubBackend) RetireAtom(_ context.Context, atomID string, validTo time.Time, superseding *atom.Atom) error {
	if s.retireErr != nil {
		return s.retireErr
	}
	predecessorActive := false
	for i := range s.existing {
		if s.existing[i].ID == atomID && s.existing[i].ValidTo == nil {
			predecessorActive = true
			break
		}
	}
	if !predecessorActive {
		return errors.New("active predecessor not found")
	}
	for _, existing := range s.existing {
		if existing.ID == superseding.ID {
			return errors.New("superseding atom ID already exists")
		}
	}
	s.inserted = append(s.inserted, *superseding)
	s.existing = append(s.existing, *superseding)
	s.mutations = append(s.mutations, "insert:"+superseding.ID)
	s.retired = append(s.retired, atomID)
	s.retiredAt[atomID] = validTo
	s.mutations = append(s.mutations, "retire:"+atomID)
	for i := range s.existing {
		if s.existing[i].ID == atomID {
			s.existing[i].ValidTo = timePointer(validTo)
		}
	}
	return nil
}

func TestStubBackend_RetireAtomRejectsRetiredPredecessorWithoutMutation(t *testing.T) {
	backend := newStubBackend()
	retiredAt := testNow.Add(-time.Hour)
	predecessor := makeAtom("old", "proj", "user", "drink", "coffee")
	predecessor.ValidTo = &retiredAt
	backend.existing = []atom.Atom{predecessor}
	replacement := makeAtom("new", "proj", "user", "drink", "tea")

	err := backend.RetireAtom(context.Background(), predecessor.ID, testNow, &replacement)

	require.Error(t, err)
	assert.Len(t, backend.existing, 1)
	assert.Empty(t, backend.inserted)
	assert.Empty(t, backend.retired)
}

// ── stub extractor ────────────────────────────────────────────────────────────

type stubExtractor struct {
	atoms        []atom.Atom
	err          error
	sessionDates []time.Time
}

func (s *stubExtractor) Extract(_ context.Context, _ string, sessionDates ...time.Time) ([]atom.Atom, error) {
	s.sessionDates = append(s.sessionDates, sessionDates...)
	return s.atoms, s.err
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestWorker_ProcessesFreshAtom(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-1", MemoryID: "mem-1", Project: "proj"}}
	backend.memories["mem-1"] = &types.Memory{ID: "mem-1", Content: "I prefer dark chocolate."}

	ext := &stubExtractor{atoms: []atom.Atom{
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "dark chocolate", Statement: "The user prefers dark chocolate.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	w := atom.NewWorker(backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Process one batch directly (not via Run which blocks).
	// Use the exported processBatch-equivalent via a single-tick test:
	// We drive it by calling Run with a very short tick, but since we can't
	// export processBatch, we tick once through a controlled context.
	// Instead, use a 1ms poll interval to get one tick before the context times out.
	w2 := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	_ = w
	go w2.Run(ctx)
	<-ctx.Done()

	// Allow the goroutine to finish processing.
	time.Sleep(50 * time.Millisecond)

	assert.NotEmpty(t, backend.inserted, "expected at least one atom inserted")
	assert.Equal(t, "proj", backend.inserted[0].Project)
	assert.Equal(t, "mem-1", backend.inserted[0].ProvenanceMemoryID)
}

func TestWorker_SupersessionRetiresThenInserts(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-1", MemoryID: "mem-1", Project: "proj"}}
	backend.memories["mem-1"] = &types.Memory{ID: "mem-1", Content: "I changed my mind.", CreatedAt: testNow}

	// Existing atom for same subject+predicate but different value.
	existing := makeAtom("existing-1", "proj", "the user", "prefers", "coffee")
	existing.ObservedAt = timePointer(testNow.Add(-time.Hour))
	backend.existing = []atom.Atom{existing}

	ext := &stubExtractor{atoms: []atom.Atom{
		// New value for same predicate → triggers supersession.
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "tea", Statement: "The user prefers tea.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()

	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, backend.retired, "existing-1", "expected old atom to be retired")
	require.NotEmpty(t, backend.inserted)
	assert.Equal(t, "tea", backend.inserted[0].Value)
	assert.Equal(t, "existing-1", backend.inserted[0].Supersedes)
}

func TestWorker_MarkJobCompleteOnSuccess(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-42", MemoryID: "mem-2", Project: "proj"}}
	backend.memories["mem-2"] = &types.Memory{ID: "mem-2", Content: "some text"}
	ext := &stubExtractor{atoms: []atom.Atom{}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)

	jobErr, seen := backend.completedJobs["job-42"]
	assert.True(t, seen, "job should be marked complete")
	assert.NoError(t, jobErr)
}

func TestWorkerThreadsCreatedAtWhenValidFromIsNil(t *testing.T) {
	backend := newStubBackend()
	createdAt := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	backend.jobs = []atom.ExtractionJob{{ID: "job-observed", MemoryID: "mem-observed", Project: "proj"}}
	backend.memories["mem-observed"] = &types.Memory{
		ID:        "mem-observed",
		Content:   "I prefer mint tea.",
		CreatedAt: createdAt,
	}
	ext := &stubExtractor{atoms: []atom.Atom{
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "mint tea", Statement: "The user prefers mint tea.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)

	require.NotEmpty(t, backend.inserted)
	require.Equal(t, []time.Time{createdAt}, ext.sessionDates)
}

func TestWorkerPrefersMemoryValidFromAsSessionDate(t *testing.T) {
	backend := newStubBackend()
	createdAt := time.Date(2026, 7, 11, 3, 30, 0, 0, time.UTC)
	validFrom := time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)
	backend.jobs = []atom.ExtractionJob{{ID: "job-valid-from", MemoryID: "mem-valid-from", Project: "proj"}}
	backend.memories["mem-valid-from"] = &types.Memory{
		ID: "mem-valid-from", Content: "I attended a meetup.", CreatedAt: createdAt, ValidFrom: &validFrom,
	}
	ext := &stubExtractor{atoms: []atom.Atom{{
		Type: atom.TypeEvent, Subject: "the user", Predicate: "attended", Value: "meetup",
		Statement: "The user attended a meetup.", Scope: atom.ScopeGlobal, Confidence: 0.9,
	}}}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	}).Run(ctx)
	<-ctx.Done()

	require.Equal(t, []time.Time{validFrom}, ext.sessionDates)
}

func TestB1CorruptionProbeRepeatedIngestPreservesExistingRows(t *testing.T) {
	backend := newStubBackend()
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	preExisting := atom.Atom{
		ID: "existing-1", Project: "proj", Type: atom.TypeEvent,
		Subject: "the user", Predicate: "attended", Value: "Go meetup",
		Statement: "On 2026-07-04, the user attended a Go meetup.",
		Scope:     atom.ScopeGlobal, Confidence: 0.9,
		ValidFrom:  timePointer(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
		ObservedAt: timePointer(createdAt),
	}
	backend.existing = []atom.Atom{preExisting}
	backend.jobs = []atom.ExtractionJob{
		{ID: "first-pass", MemoryID: "fixture", Project: "proj"},
		{ID: "second-pass", MemoryID: "fixture", Project: "proj"},
	}
	backend.memories["fixture"] = &types.Memory{
		ID: "fixture", Content: "On 2026-07-04, I attended a Go meetup.", CreatedAt: createdAt,
	}
	ext := &stubExtractor{atoms: []atom.Atom{{
		Type: atom.TypeEvent, Subject: "the user", Predicate: "attended", Value: "Go meetup",
		Statement: "On 2026-07-04, the user attended a Go meetup.", Scope: atom.ScopeGlobal,
		Confidence: 0.9, ValidFrom: timePointer(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
	}}}
	before, err := json.Marshal(backend.existing)
	require.NoError(t, err)
	mutationProbe := newStubBackend()
	mutationProbe.existing = append(mutationProbe.existing, preExisting)
	require.NoError(t, mutationProbe.RetireAtom(
		context.Background(),
		preExisting.ID,
		createdAt.Add(time.Hour),
		&atom.Atom{ID: "mutation-probe", Supersedes: preExisting.ID},
	))
	mutated, err := json.Marshal(mutationProbe.existing)
	require.NoError(t, err)
	require.NotEqual(t, before, mutated, "probe backend must expose persisted row mutations")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	worker := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go worker.Run(ctx)
	<-ctx.Done()
	time.Sleep(20 * time.Millisecond)

	after, err := json.Marshal(backend.existing)
	require.NoError(t, err)
	assert.Equal(t, before, after, "pre-existing atom rows must remain byte-identical")
	assert.Empty(t, backend.retired, "repeat ingestion must not retire pre-existing atoms")
	assert.Empty(t, backend.inserted, "exact duplicates must not create replacement rows")
	assert.Contains(t, backend.completedJobs, "first-pass")
	assert.Contains(t, backend.completedJobs, "second-pass")
}

func TestWorker_SupersessionUsesAssertionTimeAndInsertThenRetire(t *testing.T) {
	backend := newStubBackend()
	assertedAt := time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)
	backend.jobs = []atom.ExtractionJob{{ID: "job-assertion", MemoryID: "mem-assertion", Project: "proj"}}
	backend.memories["mem-assertion"] = &types.Memory{
		ID: "mem-assertion", Content: "Tea now.", CreatedAt: assertedAt,
	}
	predecessor := makeAtom("old", "proj", "the user", "prefers", "coffee")
	predecessor.ObservedAt = timePointer(assertedAt.Add(-time.Hour))
	backend.existing = []atom.Atom{predecessor}
	ext := &stubExtractor{atoms: []atom.Atom{
		makeAtom("new", "", "the user", "prefers", "tea"),
	}}

	runWorkerOnce(t, backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})

	require.Equal(t, []string{"insert:new", "retire:old"}, backend.mutations)
	assert.Equal(t, assertedAt, backend.retiredAt["old"])
	require.Len(t, backend.inserted, 1)
	assert.Equal(t, "old", backend.inserted[0].Supersedes)
}

func TestWorker_DryRunInsertsWithoutLinksOrRetirement(t *testing.T) {
	backend := newStubBackend()
	assertedAt := time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)
	backend.jobs = []atom.ExtractionJob{{ID: "job-audit", MemoryID: "mem-audit", Project: "proj"}}
	backend.memories["mem-audit"] = &types.Memory{ID: "mem-audit", Content: "Tea now.", CreatedAt: assertedAt}
	predecessor := makeAtom("old", "proj", "the user", "prefers", "coffee")
	predecessor.ObservedAt = timePointer(assertedAt.Add(-time.Hour))
	backend.existing = []atom.Atom{predecessor}
	ext := &stubExtractor{atoms: []atom.Atom{makeAtom("new", "", "the user", "prefers", "tea")}}

	runWorkerOnce(t, backend, ext, atom.WorkerConfig{
		Projects:           []string{"proj"},
		SupersessionDryRun: true,
	})

	assert.Empty(t, backend.retired)
	require.Len(t, backend.inserted, 1)
	assert.Empty(t, backend.inserted[0].Supersedes)
}

func TestWorker_RetirementFailureMarksJobFailed(t *testing.T) {
	backend := newStubBackend()
	backend.retireErr = errors.New("retirement failed")
	backend.jobs = []atom.ExtractionJob{{ID: "job-failure", MemoryID: "mem-failure", Project: "proj"}}
	backend.memories["mem-failure"] = &types.Memory{ID: "mem-failure", Content: "Tea now.", CreatedAt: testNow}
	predecessor := makeAtom("old", "proj", "the user", "prefers", "coffee")
	predecessor.ObservedAt = timePointer(testNow.Add(-time.Hour))
	backend.existing = []atom.Atom{predecessor}
	ext := &stubExtractor{atoms: []atom.Atom{makeAtom("new", "", "the user", "prefers", "tea")}}

	runWorkerOnce(t, backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})

	require.Error(t, backend.completedJobs["job-failure"])
	assert.Empty(t, backend.inserted, "atomic persistence must not leave the replacement behind")
}

func TestWorker_SupersedingIDConflictLeavesPredecessorActive(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-conflict", MemoryID: "mem-conflict", Project: "proj"}}
	backend.memories["mem-conflict"] = &types.Memory{ID: "mem-conflict", Content: "Tea now.", CreatedAt: testNow}
	predecessor := makeAtom("old", "proj", "the user", "prefers", "coffee")
	predecessor.ObservedAt = timePointer(testNow.Add(-time.Hour))
	conflicting := makeAtom("new", "proj", "unrelated", "key", "unchanged")
	backend.existing = []atom.Atom{predecessor, conflicting}
	beforeConflict, err := json.Marshal(conflicting)
	require.NoError(t, err)
	ext := &stubExtractor{atoms: []atom.Atom{makeAtom("new", "", "the user", "prefers", "tea")}}

	runWorkerOnce(t, backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})

	require.Error(t, backend.completedJobs["job-conflict"])
	byID := atomsByID(backend.existing)
	assert.Nil(t, byID["old"].ValidTo, "an ID conflict must not retire the predecessor")
	afterConflict, err := json.Marshal(byID["new"])
	require.NoError(t, err)
	assert.Equal(t, beforeConflict, afterConflict, "the pre-existing conflicting row must remain byte-identical")
	assert.Empty(t, backend.retired)
}

func TestB2CorruptionProbeSeededCorpusAndIdempotence(t *testing.T) {
	backend := newStubBackend()
	day0 := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	day1 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	backend.existing = b2SeedAtoms(day0)
	backend.memories["fixture"] = &types.Memory{ID: "fixture", Content: "B2 fixture", CreatedAt: day2}
	ext := &stubExtractor{atoms: b2CandidateAtoms(day1, day2)}

	backend.jobs = []atom.ExtractionJob{{ID: "pass-1", MemoryID: "fixture", Project: "proj"}}
	runWorkerOnce(t, backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})

	assert.ElementsMatch(t, []string{"pref-old", "pref-middle", "status-old", "status-running", "mode-current"}, backend.retired,
		"the probe must have zero false invalidations and zero missed true invalidations")
	byID := atomsByID(backend.existing)
	for _, id := range []string{
		"event-old", "recurring-event-old", "recurring-event-new", "attribute-old",
		"mixed-event", "mode-old", "timezone-current", "timezone-backfill",
	} {
		require.Nil(t, byID[id].ValidTo, "%s must remain active", id)
	}
	for _, replacementID := range []string{"pref-middle", "pref-new", "status-running", "status-done", "mode-new"} {
		replacement := byID[replacementID]
		require.NotEmpty(t, replacement.Supersedes, "%s must carry linkage", replacementID)
		predecessor, exists := byID[replacement.Supersedes]
		require.True(t, exists, "%s has a dangling supersedes reference", replacementID)
		require.NotNil(t, predecessor.ValidTo, "%s references an active predecessor", replacementID)
	}
	require.Equal(t, day1, *byID["pref-old"].ValidTo)
	require.Equal(t, day2, *byID["pref-middle"].ValidTo)
	require.Equal(t, day1, *byID["status-old"].ValidTo)
	require.Equal(t, day2, *byID["status-running"].ValidTo)

	afterPass1, err := json.Marshal(backend.existing)
	require.NoError(t, err)
	backend.jobs = []atom.ExtractionJob{{ID: "pass-2", MemoryID: "fixture", Project: "proj"}}
	runWorkerOnce(t, backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})
	afterPass2, err := json.Marshal(backend.existing)
	require.NoError(t, err)
	assert.Equal(t, afterPass1, afterPass2, "pass 2 must be byte-identical with no double retirement or re-supersession")
}

func TestB2CorruptionProbeDryRunOnlyAddsPlainInserts(t *testing.T) {
	backend := newStubBackend()
	day0 := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	day1 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	backend.existing = b2SeedAtoms(day0)
	backend.memories["fixture"] = &types.Memory{ID: "fixture", Content: "B2 fixture", CreatedAt: day2}
	backend.jobs = []atom.ExtractionJob{{ID: "dry-run", MemoryID: "fixture", Project: "proj"}}
	before, err := json.Marshal(backend.existing)
	require.NoError(t, err)

	runWorkerOnce(t, backend, &stubExtractor{atoms: b2CandidateAtoms(day1, day2)}, atom.WorkerConfig{
		Projects:           []string{"proj"},
		SupersessionDryRun: true,
	})

	afterSeedRows, err := json.Marshal(backend.existing[:len(b2SeedAtoms(day0))])
	require.NoError(t, err)
	assert.Equal(t, before, afterSeedRows, "dry-run must leave every pre-existing row byte-identical")
	assert.Empty(t, backend.retired)
	for _, inserted := range backend.inserted {
		assert.Empty(t, inserted.Supersedes, "dry-run inserts must be plain, unlinked atoms")
	}
}

func b2SeedAtoms(observedAt time.Time) []atom.Atom {
	preference := makeAtom("pref-old", "proj", "user", "drink", "coffee")
	preference.ObservedAt = &observedAt
	event := makeAtom("event-old", "proj", "release", "deployed", "v1")
	event.Type = atom.TypeEvent
	event.ObservedAt = &observedAt
	recurringEvent := makeAtom("recurring-event-old", "proj", "user", "attended", "weekly meetup")
	recurringEvent.Type = atom.TypeEvent
	recurringEvent.ValidFrom = timePointer(observedAt)
	recurringEvent.ObservedAt = &observedAt
	attribute := makeAtom("attribute-old", "proj", "service", "region", "east")
	attribute.Type = atom.TypeAttribute
	attribute.Confidence = 1
	attribute.ObservedAt = &observedAt
	status := makeAtom("status-old", "proj", "release", "status", "queued")
	status.Type = atom.TypeStatusChange
	status.ObservedAt = &observedAt
	mixedEvent := makeAtom("mixed-event", "proj", "release", "status", "announced")
	mixedEvent.Type = atom.TypeEvent
	mixedEvent.ObservedAt = &observedAt
	modeOld := makeAtom("mode-old", "proj", "service", "mode", "legacy")
	modeOld.Type = atom.TypeFact
	modeOld.ObservedAt = timePointer(observedAt.Add(-24 * time.Hour))
	modeCurrent := makeAtom("mode-current", "proj", "service", "mode", "current")
	modeCurrent.Type = atom.TypeFact
	modeCurrent.ObservedAt = &observedAt
	timezoneCurrent := makeAtom("timezone-current", "proj", "user", "timezone", "America/New_York")
	timezoneCurrent.Type = atom.TypeProfile
	timezoneCurrent.ValidFrom = timePointer(observedAt)
	timezoneCurrent.ObservedAt = &observedAt
	return []atom.Atom{
		preference, event, recurringEvent, attribute, status, mixedEvent,
		modeCurrent, modeOld, timezoneCurrent,
	}
}

func b2CandidateAtoms(day1, day2 time.Time) []atom.Atom {
	preferenceMiddle := makeAtom("pref-middle", "", "user", "drink", "tea")
	preferenceMiddle.ObservedAt = &day1
	preference := makeAtom("pref-new", "", "user", "drink", "water")
	preference.ObservedAt = &day2
	lowConfidence := makeAtom("attribute-low", "", "service", "region", "west")
	lowConfidence.Type = atom.TypeAttribute
	lowConfidence.Confidence = 0.79
	lowConfidence.ObservedAt = &day1
	recurringEvent := makeAtom("event-new", "", "release", "deployed", "v2")
	recurringEvent.Type = atom.TypeEvent
	recurringEvent.ObservedAt = &day1
	sameValueRecurringEvent := makeAtom("recurring-event-new", "", "user", "attended", "weekly meetup")
	sameValueRecurringEvent.Type = atom.TypeEvent
	sameValueRecurringEvent.ValidFrom = timePointer(day1)
	sameValueRecurringEvent.ObservedAt = &day1
	running := makeAtom("status-running", "", "release", "status", "running")
	running.Type = atom.TypeStatusChange
	running.ObservedAt = &day1
	done := makeAtom("status-done", "", "release", "status", "done")
	done.Type = atom.TypeStatusChange
	done.ObservedAt = &day2
	mode := makeAtom("mode-new", "", "service", "mode", "future")
	mode.Type = atom.TypeFact
	mode.ObservedAt = &day2
	timezoneBackfill := makeAtom("timezone-backfill", "", "user", "timezone", "UTC")
	timezoneBackfill.Type = atom.TypeProfile
	timezoneBackfill.ObservedAt = timePointer(day1.Add(-48 * time.Hour))
	return []atom.Atom{
		done, recurringEvent, sameValueRecurringEvent, lowConfidence, preferenceMiddle,
		preference, running, mode, timezoneBackfill,
	}
}

func atomsByID(atoms []atom.Atom) map[string]atom.Atom {
	result := make(map[string]atom.Atom, len(atoms))
	for _, candidate := range atoms {
		result[candidate.ID] = candidate
	}
	return result
}

func runWorkerOnce(t *testing.T, backend *stubBackend, ext *stubExtractor, cfg atom.WorkerConfig) {
	t.Helper()
	cfg.PollInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		atom.NewWorker(backend, ext, cfg).Run(ctx)
	}()
	<-ctx.Done()
	<-done
}

func TestB1CorruptionProbeSameValueDifferentDateEventsBothStored(t *testing.T) {
	backend := newStubBackend()
	firstDate := time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)
	secondDate := time.Date(2023, 5, 16, 0, 0, 0, 0, time.UTC)
	backend.existing = []atom.Atom{{
		ID: "first-event", Project: "proj", Type: atom.TypeEvent,
		Subject: "the user", Predicate: "attended", Value: "weekly meetup",
		Statement: "On 2023-05-09, the user attended the weekly meetup.",
		Scope:     atom.ScopeGlobal, Confidence: 0.9, ValidFrom: &firstDate,
	}}
	backend.jobs = []atom.ExtractionJob{{ID: "second-event-job", MemoryID: "second-event-memory", Project: "proj"}}
	backend.memories["second-event-memory"] = &types.Memory{
		ID: "second-event-memory", Content: "On 2023-05-16, I attended the weekly meetup.",
		CreatedAt: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), ValidFrom: &secondDate,
	}
	ext := &stubExtractor{atoms: []atom.Atom{{
		Type: atom.TypeEvent, Subject: "the user", Predicate: "attended", Value: "weekly meetup",
		Statement: "On 2023-05-16, the user attended the weekly meetup.",
		Scope:     atom.ScopeGlobal, Confidence: 0.9, ValidFrom: &secondDate,
	}}}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	}).Run(ctx)
	<-ctx.Done()

	require.Len(t, backend.existing, 2, "both dated event occurrences must be stored")
	assert.Equal(t, firstDate, *backend.existing[0].ValidFrom)
	assert.Equal(t, secondDate, *backend.existing[1].ValidFrom)
	assert.Empty(t, backend.retired, "storing a recurrence must not mutate the earlier event")
}

func timePointer(value time.Time) *time.Time {
	return &value
}
