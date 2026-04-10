// Package mcp wires the SearchEngine to the MCP protocol layer.
package mcp

import (
	"context"
	"sync"

	"github.com/petersimmons1972/engram/internal/search"
)

// EngineHandle wraps a SearchEngine so the pool can manage its lifecycle.
type EngineHandle struct {
	Engine *search.SearchEngine
}

// EngineFactory creates a new SearchEngine for a project.
type EngineFactory func(ctx context.Context, project string) (*EngineHandle, error)

// EnginePool lazily creates and caches one SearchEngine per project.
// Safe for concurrent use.
type EnginePool struct {
	mu      sync.Mutex
	engines map[string]*EngineHandle
	factory EngineFactory
}

// NewEnginePool creates an EnginePool using factory to construct missing engines.
func NewEnginePool(factory EngineFactory) *EnginePool {
	return &EnginePool{
		engines: make(map[string]*EngineHandle),
		factory: factory,
	}
}

// Get returns the cached engine for project, creating one via factory if needed.
func (p *EnginePool) Get(ctx context.Context, project string) (*EngineHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if h, ok := p.engines[project]; ok {
		return h, nil
	}
	h, err := p.factory(ctx, project)
	if err != nil {
		return nil, err
	}
	p.engines[project] = h
	return h, nil
}

// Close stops all cached engines.
func (p *EnginePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range p.engines {
		if h.Engine != nil {
			h.Engine.Close()
		}
	}
}
