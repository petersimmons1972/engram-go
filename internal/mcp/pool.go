// Package mcp wires the SearchEngine to the MCP protocol layer.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"golang.org/x/sync/singleflight"
)

// maxPoolSize is the maximum number of project engines kept in memory at once.
// When exceeded, the least-recently-used engine is evicted and its connection
// pool released. 50 projects × 2–10 PG connections each = 100–500 connections,
// well within typical PostgreSQL limits.
const maxPoolSize = 50

// EngineHandle wraps a SearchEngine so the pool can manage its lifecycle.
type EngineHandle struct {
	Engine *search.SearchEngine
}

// EngineFactory creates a new SearchEngine for a project.
type EngineFactory func(ctx context.Context, project string) (*EngineHandle, error)

// engineEntry holds a handle plus its last-access timestamp for LRU eviction.
// lastAccess stores UnixNano as an atomic int64 so the fast path (RLock) can
// update it without a data race.
type engineEntry struct {
	handle     *EngineHandle
	lastAccess atomic.Int64 // UnixNano; use Load/Store only
}

// EnginePool lazily creates and caches one SearchEngine per project.
// It enforces a maximum pool size by evicting the least-recently-used engine
// when the cap is reached. Safe for concurrent use.
type EnginePool struct {
	mu      sync.RWMutex
	engines map[string]*engineEntry
	factory EngineFactory
	sfg     singleflight.Group
	closed  atomic.Bool
}

// NewEnginePool creates an EnginePool using factory to construct missing engines.
func NewEnginePool(factory EngineFactory) *EnginePool {
	return &EnginePool{
		engines: make(map[string]*engineEntry),
		factory: factory,
	}
}

// Get returns the cached engine for project, creating one via factory if needed.
// If the pool is at capacity, the least-recently-used engine is evicted first.
//
// Uses double-checked locking so the slow factory path (PostgreSQL connection +
// migrations) runs outside the mutex, preventing contention across projects (#31).
func (p *EnginePool) Get(ctx context.Context, project string) (*EngineHandle, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("engine pool is closed")
	}
	// Normalize before using as a cache key so " foo" and "foo" don't alias
	// to different pool entries that point to the same backend (#143).
	project = strings.ToLower(strings.TrimSpace(project))
	if len(project) > 128 {
		return nil, fmt.Errorf("project name too long (%d chars, max 128)", len(project))
	}

	// Fast path: read lock is sufficient when the engine already exists.
	// lastAccess is updated atomically so multiple goroutines can safely
	// record their access time while holding only RLock.
	p.mu.RLock()
	if e, ok := p.engines[project]; ok {
		e.lastAccess.Store(time.Now().UnixNano())
		p.mu.RUnlock()
		return e.handle, nil
	}
	p.mu.RUnlock()

	// Slow path: use singleflight to ensure only one goroutine runs the
	// factory for a given project at a time. All concurrent callers for the
	// same project share the result, preventing TOCTOU races and duplicate
	// backend connection pools.
	v, err, _ := p.sfg.Do(project, func() (any, error) {
		// Re-check under read lock in case a previous singleflight call (from
		// before pool startup) already inserted this project.
		p.mu.RLock()
		if e, ok := p.engines[project]; ok {
			p.mu.RUnlock()
			return e.handle, nil
		}
		p.mu.RUnlock()

		// Bound the factory with a 10s timeout so a slow DB migration (schema
		// init, connection pool setup) cannot block the caller indefinitely.
		// singleflight does NOT cache errors, so a timeout here allows the next
		// Get() call to retry the factory rather than returning a stale error.
		initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
		defer initCancel()
		h, err := p.factory(initCtx, project)
		if err != nil {
			return nil, err
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		if e, ok := p.engines[project]; ok {
			// Another singleflight group (from a different key race) won — discard.
			if h != nil && h.Engine != nil {
				h.Engine.Close()
			}
			e.lastAccess.Store(time.Now().UnixNano())
			return e.handle, nil
		}
		for len(p.engines) >= maxPoolSize {
			p.evictLRULocked()
		}
		entry := &engineEntry{handle: h}
		entry.lastAccess.Store(time.Now().UnixNano())
		p.engines[project] = entry
		return h, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*EngineHandle), nil //nolint:errcheck
}

// evictLRULocked removes the engine with the oldest lastAccess time.
// Caller must hold p.mu.
func (p *EnginePool) evictLRULocked() {
	var lruKey string
	lruNano := int64(math.MaxInt64) // #131: init to max so any real value wins
	for k, e := range p.engines {
		nano := e.lastAccess.Load()
		if nano < lruNano || (nano == lruNano && k < lruKey) {
			lruKey = k
			lruNano = nano
		}
	}
	if lruKey == "" {
		return
	}
	e := p.engines[lruKey]
	if e.handle != nil && e.handle.Engine != nil {
		e.handle.Engine.Close()
	}
	delete(p.engines, lruKey)
}

// Close stops all cached engines and marks the pool as closed.
// Subsequent calls to Get will return an error.
func (p *EnginePool) Close() {
	p.closed.Store(true)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.engines {
		if e.handle != nil && e.handle.Engine != nil {
			e.handle.Engine.Close()
		}
	}
}

// WarmProjects pre-initializes project engines in the pool, best-effort.
// Errors are logged but not returned. Respects ctx cancellation.
// concurrency controls how many projects are initialized in parallel;
// 0 or negative uses a default of 3.
func (p *EnginePool) WarmProjects(ctx context.Context, projects []string, concurrency int) {
	if len(projects) == 0 {
		return
	}
	if concurrency <= 0 {
		concurrency = 3
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, proj := range projects {
		proj := proj
		select {
		case <-ctx.Done():
			break
		default:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := p.Get(ctx, proj); err != nil {
				slog.Warn("pool pre-warm: Get failed", "project", proj, "err", err)
			}
		}()
	}
	wg.Wait()
}
