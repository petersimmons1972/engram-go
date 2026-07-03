package llmclient

// Internal white-box test for the ollaClient mutex fix.
// Verifies that concurrent resolvedModel + resetModel calls do not race.
// Must be run with -race to catch the bug the sync.Once variant had.

import (
	"context"
	"net/http"
	"sync"
	"testing"
)

// TestResetModelNoRace spawns concurrent goroutines that call resolvedModel
// and resetModel in a tight loop. With the old sync.Once implementation
// (c.modelOnce = sync.Once{}) this would be flagged by the race detector
// because it writes to a struct value field while another goroutine may be
// reading it inside sync.Once.Do. With the mutex-based implementation the
// test must complete cleanly under -race.
func TestResetModelNoRace(t *testing.T) {
	c := &ollaClient{
		host:    "http://127.0.0.1:1", // unreachable; pickModel will return ""
		timeout: 0,
		client:  &http.Client{Timeout: 0},
	}

	const workers = 8
	const iters = 500

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Half the goroutines call resolvedModel; the other half call resetModel.
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = c.resolvedModel(context.Background())
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				c.resetModel()
			}
		}()
	}

	wg.Wait()
	// If we reach here without the race detector firing, the fix is correct.
}
