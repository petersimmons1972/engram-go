package ingestqueue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusDone       JobStatus = "done"
	StatusFailed     JobStatus = "failed"
)

type Job struct {
	ID      string
	Project string
	Work    func(ctx context.Context) error
}

type JobResult struct {
	JobID     string
	Status    JobStatus
	Error     string
	StartedAt time.Time
	DoneAt    time.Time
}

type Config struct {
	Depth   int
	Workers int
}

var ErrQueueFull = fmt.Errorf("ingestion queue full — retry after current jobs complete")

var (
	resultTTL              = 5 * time.Minute
	resultEvictionInterval = time.Minute
)

type Queue struct {
	ch       chan *Job
	results  sync.Map
	inflight atomic.Int64
	wg       sync.WaitGroup
}

func New(ctx context.Context, cfg Config) *Queue {
	if cfg.Depth <= 0 {
		cfg.Depth = 64
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	q := &Queue{ch: make(chan *Job, cfg.Depth)}
	q.wg.Add(1)
	go q.evictLoop(ctx)
	for i := 0; i < cfg.Workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx)
	}
	return q
}

func (q *Queue) Wait() { q.wg.Wait() }

func (q *Queue) Enqueue(job *Job) error {
	q.results.Store(job.ID, &JobResult{JobID: job.ID, Status: StatusPending})
	select {
	case q.ch <- job:
		return nil
	default:
		q.results.Delete(job.ID)
		return ErrQueueFull
	}
}

func (q *Queue) Status(jobID string) *JobResult {
	v, ok := q.results.Load(jobID)
	if !ok {
		return nil
	}
	jr, _ := v.(*JobResult)
	return jr
}

func (q *Queue) Depth() int { return len(q.ch) }
func (q *Queue) worker(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-q.ch:
			if !ok {
				return
			}
			q.inflight.Add(1)
			q.run(ctx, job)
			q.inflight.Add(-1)
		}
	}
}

func (q *Queue) run(ctx context.Context, job *Job) {
	startedAt := time.Now()
	err := job.Work(ctx)
	finalResult := JobResult{
		JobID:     job.ID,
		StartedAt: startedAt,
		DoneAt:    time.Now(),
	}
	if err != nil {
		finalResult.Status = StatusFailed
		finalResult.Error = err.Error()
		slog.Warn("ingest job failed", "job_id", job.ID, "project", job.Project, "err", err)
	} else {
		finalResult.Status = StatusDone
	}
	q.results.Store(job.ID, &finalResult)
}

func (q *Queue) evictLoop(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(resultEvictionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.evictCompletedResults(time.Now())
		}
	}
}

func (q *Queue) evictCompletedResults(now time.Time) {
	q.results.Range(func(key, value any) bool {
		result, ok := value.(*JobResult)
		if !ok {
			return true
		}
		if result.Status != StatusDone && result.Status != StatusFailed {
			return true
		}
		if result.DoneAt.IsZero() || now.Sub(result.DoneAt) < resultTTL {
			return true
		}
		q.results.Delete(key)
		return true
	})
}
