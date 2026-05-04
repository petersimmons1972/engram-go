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

func (q *Queue) Depth() int    { return len(q.ch) }
func (q *Queue) Inflight() int { return int(q.inflight.Load()) }

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
	r := &JobResult{JobID: job.ID, Status: StatusProcessing, StartedAt: time.Now()}
	q.results.Store(job.ID, r)
	err := job.Work(ctx)
	r.DoneAt = time.Now()
	if err != nil {
		r.Status = StatusFailed
		r.Error = err.Error()
		slog.Warn("ingest job failed", "job_id", job.ID, "project", job.Project, "err", err)
	} else {
		r.Status = StatusDone
	}
	q.results.Store(job.ID, r)
}
