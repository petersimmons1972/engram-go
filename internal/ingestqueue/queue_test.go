package ingestqueue_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/ingestqueue"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 16, Workers: 2})

	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("job-%d", i)
		err := q.Enqueue(&ingestqueue.Job{
			ID: id, Project: "test",
			Work: func(ctx context.Context) error { done <- struct{}{}; return nil },
		})
		if err != nil {
			t.Fatalf("Enqueue %s: %v", id, err)
		}
	}
	timeout := time.After(2 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("timed out waiting for jobs")
		}
	}
}

func TestQueue_QueueFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	block := make(chan struct{})
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 2, Workers: 1})

	// Fill the 1 worker slot with a blocking job.
	started := make(chan struct{}, 1)
	_ = q.Enqueue(&ingestqueue.Job{
		ID: "worker-holder", Project: "test",
		Work: func(ctx context.Context) error {
			started <- struct{}{} // signal worker is occupied
			<-block
			return nil
		},
	})
	<-started // wait until worker is definitely busy

	// Now fill the 2-slot channel.
	for i := 0; i < 2; i++ {
		err := q.Enqueue(&ingestqueue.Job{
			ID: fmt.Sprintf("filler-%d", i), Project: "test",
			Work: func(ctx context.Context) error { return nil },
		})
		if err != nil {
			t.Fatalf("filler %d: unexpected error: %v", i, err)
		}
	}

	// Channel is full AND worker is busy — next enqueue must fail.
	err := q.Enqueue(&ingestqueue.Job{
		ID: "overflow", Project: "test",
		Work: func(ctx context.Context) error { return nil },
	})
	if err != ingestqueue.ErrQueueFull {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}
	close(block)
}

func TestQueue_FailedJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 4, Workers: 1})

	done := make(chan struct{}, 1)
	_ = q.Enqueue(&ingestqueue.Job{
		ID: "fail-job", Project: "test",
		Work: func(ctx context.Context) error {
			done <- struct{}{}
			return fmt.Errorf("deliberate failure")
		},
	})
	<-done
	time.Sleep(20 * time.Millisecond)

	r := q.Status("fail-job")
	if r == nil || r.Status != ingestqueue.StatusFailed {
		t.Fatalf("want StatusFailed, got %v", r)
	}
	if r.Error == "" {
		t.Error("want non-empty Error field")
	}
}

func TestQueue_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 4, Workers: 2})
	cancel()
	done := make(chan struct{})
	go func() { q.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers did not stop after ctx cancel")
	}
}
