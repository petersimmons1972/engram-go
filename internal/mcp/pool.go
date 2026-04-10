// Package mcp wires the SearchEngine to the MCP protocol layer.
package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
)

// maxPoolSize is the maximum number of project engines kept in memory at once.
// When exceeded, the least-recently-used engine is evicted and its connection
// pool released. 50 projects × ~10 PG connections each = 500 max connections,
// well within typical PostgreSQL limits.
const maxPoolSize = 50

// EngineHandle wraps a SearchEngine so the pool can manage its lifecycle.
type EngineHandle struct {
	Engine *search.SearchEngine
}

// EngineFactory creates a new SearchEngine for a project.
type EngineFactory func(ctx context.Context, project string) (*EngineHandle, error)

// engineEntry holds a handle plus its last-access timestamp for LRU eviction.
type engineEntry struct {
	handle     *EngineHandle
	lastAccess time.Time
}

// EnginePool lazily creates and caches one SearchEngine per project.
// It enforces a maximum pool size by evicting the least-recently-used engine
// when the cap is reached. Safe for concurrent use.
type EnginePool struct {
	mu      sync.RWMutex
	engines map[string]*engineEntry
	factory EngineFactory
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
	if len(project) > 128 {
		return nil, fmt.Errorf("project name too long (%d chars, max 128)", len(project))
	}

	// Fast path: read lock is sufficient when the engine already exists.
	p.mu.RLock()
	if e, ok := p.engines[project]; ok {
		e.lastAccess = time.Now()
		p.mu.RUnlock()
		return e.handle, nil
	}
	p.mu.RUnlock()

	// Slow path: create engine OUTSIDE the lock so other projects are not blocked
	// during the PostgreSQL connection + migration phase.
	h, err := p.factory(ctx, project)
	if err != nil {
		return nil, err
	}

	// Write lock only to insert. Check again in case a concurrent caller already
	// created this project's engine while we were in factory.
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.engines[project]; ok {
		// Race: another goroutine won — discard the engine we just created.
		if h != nil && h.Engine != nil {
			h.Engine.Close()
		}
		e.lastAccess = time.Now()
		return e.handle, nil
	}
	if len(p.engines) >= maxPoolSize {
		p.evictLRULocked()
	}
	p.engines[project] = &engineEntry{handle: h, lastAccess: time.Now()}
	return h, nil
}

// evictLRULocked removes the engine with the oldest lastAccess time.
// Caller must hold p.mu.
func (p *EnginePool) evictLRULocked() {
	var lruKey string
	var lruTime time.Time
	first := true
	for k, e := range p.engines {
		if first || e.lastAccess.Before(lruTime) {
			lruKey = k
			lruTime = e.lastAccess
			first = false
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

// Close stops all cached engines.
func (p *EnginePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.engines {
		if e.handle != nil && e.handle.Engine != nil {
			e.handle.Engine.Close()
		}
	}
}
