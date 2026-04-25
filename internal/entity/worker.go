package entity

import (
	"context"
	"log/slog"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// ExtractionJob is the minimal job descriptor the Worker needs from the database.
// It mirrors db.ExtractionJob exactly so callers can convert between them without
// an import of the db package from within the entity package (which would create
// an import cycle, since db imports entity for the Entity type).
type ExtractionJob struct {
	ID       string
	MemoryID string
	Project  string
}

// WorkerBackend is the narrow database interface required by the Worker.
// It is satisfied by the dbAdapter type in cmd/engram/main.go, and by test
// stubs, without requiring the entity package to import the db package
// (which would create an import cycle).
//
// Note: UpsertEntity returns (id string, err error) to match db.Backend.
type WorkerBackend interface {
	ClaimExtractionJobs(ctx context.Context, project string, limit int) ([]ExtractionJob, error)
	CompleteExtractionJob(ctx context.Context, jobID string, err error) error
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
	GetEntitiesByProject(ctx context.Context, project string) ([]Entity, error)
	UpsertEntity(ctx context.Context, e *Entity) (string, error)
}

// WorkerConfig controls how the extraction worker polls the database.
type WorkerConfig struct {
	// PollInterval is how often the worker checks for new extraction jobs.
	// Defaults to 5 seconds if zero.
	PollInterval time.Duration
	// BatchSize is the maximum number of jobs claimed per poll tick.
	// Defaults to 10 if zero.
	BatchSize int
	// Projects is the list of project names to poll.
	Projects []string
}

// Worker polls the database for pending entity extraction jobs and processes them.
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

// Run blocks until ctx is cancelled, polling for extraction jobs on each tick.
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

// safeProcessBatch wraps processBatch with per-iteration panic recovery (#247).
// A panic logs an error and sleeps 1s so the loop can continue rather than
// killing the worker goroutine permanently.
func (w *Worker) safeProcessBatch(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("entity worker panic — will retry next tick",
				"panic", r)
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
		jobs, err := w.db.ClaimExtractionJobs(ctx, project, w.config.BatchSize)
		if err != nil {
			slog.Warn("entity worker: claim jobs failed", "project", project, "err", err)
			continue
		}
		for _, job := range jobs {
			w.processJob(ctx, job)
		}
	}
}

// processJob processes a single extraction job:
//  1. Fetches the memory from the database.
//  2. Extracts entities and relations via the Extractor.
//  3. Deduplicates candidates against existing project entities.
//  4. Upserts merged and fresh entities.
//  5. Marks the job complete (or failed).
func (w *Worker) processJob(ctx context.Context, job ExtractionJob) {
	mem, err := w.db.GetMemory(ctx, job.MemoryID)
	if err != nil {
		slog.Warn("entity worker: fetch memory failed", "job", job.ID, "memory", job.MemoryID, "err", err)
		if cerr := w.db.CompleteExtractionJob(ctx, job.ID, err); cerr != nil {
			slog.Warn("entity worker: mark job failed (GetMemory path)", "job", job.ID, "err", cerr)
		}
		return
	}

	candidates, relations, err := w.ext.Extract(ctx, mem.Content)
	if err != nil {
		slog.Warn("entity worker: extraction failed", "job", job.ID, "err", err)
		if cerr := w.db.CompleteExtractionJob(ctx, job.ID, err); cerr != nil {
			slog.Warn("entity worker: mark job failed (Extract path)", "job", job.ID, "err", cerr)
		}
		return
	}

	existing, err := w.db.GetEntitiesByProject(ctx, job.Project)
	if err != nil {
		slog.Warn("entity worker: failed to load existing entities, skipping deduplication", "project", job.Project, "err", err)
		return
	}
	merged, fresh := Deduplicate(existing, candidates)

	for i := range merged {
		merged[i].Project = job.Project
		if _, err := w.db.UpsertEntity(ctx, &merged[i]); err != nil {
			slog.Warn("entity worker: upsert merged entity failed", "name", merged[i].Name, "err", err)
		}
	}
	for i := range fresh {
		fresh[i].Project = job.Project
		if _, err := w.db.UpsertEntity(ctx, &fresh[i]); err != nil {
			slog.Warn("entity worker: upsert fresh entity failed", "name", fresh[i].Name, "err", err)
		}
	}

	if len(relations) > 0 {
		slog.Debug("entity worker: relations extracted (not yet persisted)",
			"job", job.ID, "count", len(relations))
	}

	if cerr := w.db.CompleteExtractionJob(ctx, job.ID, nil); cerr != nil {
		slog.Warn("entity worker: mark job complete failed", "job", job.ID, "err", cerr)
	}
}
