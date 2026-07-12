package atom

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/types"
)

// ExtractionJob is the minimal job descriptor the Worker needs from the database.
// Mirrors entity.ExtractionJob to avoid an import cycle.
type ExtractionJob struct {
	ID       string
	MemoryID string
	Project  string
}

// WorkerBackend is the narrow database interface required by the Worker.
// Satisfied by an adapter in cmd/engram/main.go and by test stubs, without
// requiring the atom package to import the db package (which would create an
// import cycle).
type WorkerBackend interface {
	ClaimAtomExtractionJobs(ctx context.Context, project string, limit int) ([]ExtractionJob, error)
	CompleteAtomExtractionJob(ctx context.Context, jobID string, err error) error
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
	GetActiveAtoms(ctx context.Context, project string, atomType string) ([]Atom, error)
	InsertAtom(ctx context.Context, a *Atom) error
	RetireAtom(ctx context.Context, atomID string, validTo time.Time, superseding *Atom) error
}

// WorkerConfig controls how the atom extraction worker polls the database.
type WorkerConfig struct {
	// PollInterval is how often the worker checks for new extraction jobs.
	// Defaults to 5 seconds if zero.
	PollInterval time.Duration
	// BatchSize is the maximum number of jobs claimed per poll tick.
	// Defaults to 10 if zero.
	BatchSize int
	// Projects is the list of project names to poll.
	Projects []string
	// SupersessionDryRun logs supersession decisions but persists candidates as
	// unlinked active atoms without retiring their predecessors.
	SupersessionDryRun bool
}

// Worker polls the database for pending atom extraction jobs and processes them.
// It is safe for concurrent use; start exactly one goroutine per Worker via Run.
type Worker struct {
	db     WorkerBackend
	ext    Extractor
	config WorkerConfig
}

// NewWorker creates a Worker. Zero values in cfg are replaced by sensible defaults.
func NewWorker(backend WorkerBackend, ext Extractor, cfg WorkerConfig) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 10
	}
	return &Worker{db: backend, ext: ext, config: cfg}
}

// Run blocks until ctx is cancelled, polling for atom extraction jobs on each tick.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.safeProcessBatch(ctx)
		}
	}
}

// safeProcessBatch wraps processBatch with per-iteration panic recovery,
// mirroring the entity worker pattern (#247).
func (w *Worker) safeProcessBatch(ctx context.Context) {
	metrics.WorkerTicks.WithLabelValues("atom").Inc()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("atom worker panic — will retry next tick",
				"panic", r)
			metrics.WorkerPanics.WithLabelValues("atom").Inc()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}()
	w.processBatch(ctx)
}

// processBatch claims and processes a batch of jobs for every configured project.
func (w *Worker) processBatch(ctx context.Context) {
	for _, project := range w.config.Projects {
		jobs, err := w.db.ClaimAtomExtractionJobs(ctx, project, w.config.BatchSize)
		if err != nil {
			slog.Warn("atom worker: claim jobs failed", "project", project, "err", err)
			continue
		}
		for _, job := range jobs {
			w.processJob(ctx, job)
		}
	}
}

// processJob processes a single atom extraction job:
//  1. Fetches the memory from the database.
//  2. Extracts atoms via the Extractor.
//  3. Loads active atoms for the project to feed deduplication.
//  4. Deduplicates and atomically persists fresh and superseding atoms.
//  5. Marks the job complete (or failed).
func (w *Worker) processJob(ctx context.Context, job ExtractionJob) {
	mem, err := w.db.GetMemory(ctx, job.MemoryID)
	if err != nil {
		slog.Warn("atom worker: fetch memory failed", "job", job.ID, "memory", job.MemoryID, "err", err)
		if cerr := w.db.CompleteAtomExtractionJob(ctx, job.ID, err); cerr != nil {
			slog.Warn("atom worker: mark job failed (GetMemory path)", "job", job.ID, "err", cerr)
		}
		return
	}

	sessionDate := mem.CreatedAt
	if mem.ValidFrom != nil {
		sessionDate = *mem.ValidFrom
	}
	candidates, err := w.ext.Extract(ctx, mem.Content, sessionDate)
	if err != nil {
		slog.Warn("atom worker: extraction failed", "job", job.ID, "err", err)
		if cerr := w.db.CompleteAtomExtractionJob(ctx, job.ID, err); cerr != nil {
			slog.Warn("atom worker: mark job failed (Extract path)", "job", job.ID, "err", cerr)
		}
		return
	}

	for i := range candidates {
		if candidates[i].ID == "" {
			candidates[i].ID = uuid.NewString()
		}
		candidates[i].Project = job.Project
		candidates[i].ProvenanceMemoryID = job.MemoryID
		if candidates[i].ObservedAt == nil {
			observedAt := sessionDate
			candidates[i].ObservedAt = &observedAt
		}
	}

	// Load all active atoms for the project to drive deduplication.
	// We fetch all types here; the dedup logic keys on (subject, predicate).
	existing, err := w.db.GetActiveAtoms(ctx, job.Project, "")
	if err != nil {
		slog.Warn("atom worker: failed to load existing atoms, skipping deduplication",
			"project", job.Project, "err", err)
		// Still insert candidates as fresh (no supersession risk).
		for i := range candidates {
			if err := w.db.InsertAtom(ctx, &candidates[i]); err != nil {
				slog.Warn("atom worker: insert atom failed (no-dedup fallback)", "err", err)
				w.completeJob(ctx, job.ID, fmt.Errorf("insert atom without deduplication: %w", err))
				return
			}
		}
		w.completeJob(ctx, job.ID, nil)
		return
	}

	result := Deduplicate(existing, candidates, time.Now().UTC())

	// Insert fresh atoms.
	for i := range result.Fresh {
		if err := w.db.InsertAtom(ctx, &result.Fresh[i]); err != nil {
			slog.Warn("atom worker: insert fresh atom failed", "subject", result.Fresh[i].Subject, "err", err)
			w.completeJob(ctx, job.ID, fmt.Errorf("insert fresh atom: %w", err))
			return
		}
	}

	// Each backend call atomically inserts the linked replacement before retiring
	// its predecessor. Readers therefore never observe an unlinked overlap.
	for _, pair := range result.Superseded {
		if pair.Old.ValidTo == nil {
			continue
		}
		newAtom := pair.New
		if w.config.SupersessionDryRun {
			slog.Info(
				"atom supersession dry run: would supersede",
				"existing_id", pair.Old.ID,
				"existing_value", pair.Old.Value,
				"candidate_id", newAtom.ID,
				"candidate_value", newAtom.Value,
			)
			newAtom.Supersedes = ""
			if err := w.db.InsertAtom(ctx, &newAtom); err != nil {
				w.completeJob(ctx, job.ID, fmt.Errorf("insert dry-run atom: %w", err))
				return
			}
			continue
		}
		if err := w.db.RetireAtom(ctx, pair.Old.ID, *pair.Old.ValidTo, &newAtom); err != nil {
			slog.Warn("atom worker: persist supersession failed", "id", pair.Old.ID, "err", err)
			w.completeJob(ctx, job.ID, fmt.Errorf("persist atom supersession: %w", err))
			return
		}
	}

	w.completeJob(ctx, job.ID, nil)
}

func (w *Worker) completeJob(ctx context.Context, jobID string, jobErr error) {
	if err := w.db.CompleteAtomExtractionJob(ctx, jobID, jobErr); err != nil {
		slog.Warn("atom worker: mark job complete failed", "job", jobID, "err", err)
	}
}
