package ingestqueue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestQueue_ConcurrentStatus_NoRace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := New(ctx, Config{Depth: 2, Workers: 1})
	started := make(chan struct{})
	release := make(chan struct{})

	if err := q.Enqueue(&Job{
		ID:      "race-job",
		Project: "test",
		Work: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	<-started

	stopReaders := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				if r := q.Status("race-job"); r != nil {
					_ = r.JobID
					_ = r.Status
					_ = r.Error
					_ = r.StartedAt
					_ = r.DoneAt
				}
			}
		}()
	}

	time.Sleep(25 * time.Millisecond)
	close(release)

	deadline := time.After(2 * time.Second)
	for {
		r := q.Status("race-job")
		if r != nil && r.Status == StatusDone {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for completed result")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	close(stopReaders)
	wg.Wait()
}

func TestQueue_ResultsEviction(t *testing.T) {
	oldTTL := resultTTL
	oldInterval := resultEvictionInterval
	resultTTL = 20 * time.Millisecond
	resultEvictionInterval = 5 * time.Millisecond
	defer func() {
		resultTTL = oldTTL
		resultEvictionInterval = oldInterval
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := New(ctx, Config{Depth: 2, Workers: 1})
	started := make(chan struct{})
	release := make(chan struct{})

	if err := q.Enqueue(&Job{
		ID:      "ttl-job",
		Project: "test",
		Work: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	<-started
	time.Sleep(50 * time.Millisecond)

	if r := q.Status("ttl-job"); r == nil {
		t.Fatal("pending job result was evicted before completion")
	}
	if got := countResults(&q.results); got != 1 {
		t.Fatalf("want 1 result while job pending, got %d", got)
	}

	close(release)

	deadline := time.After(2 * time.Second)
	for {
		r := q.Status("ttl-job")
		if r != nil && r.Status == StatusDone {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for completed result")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	deadline = time.After(2 * time.Second)
	for {
		if q.Status("ttl-job") == nil && countResults(&q.results) == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("completed job result was not evicted; count=%d", countResults(&q.results))
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func countResults(results *sync.Map) int {
	count := 0
	results.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
