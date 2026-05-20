package internal

import (
	"sync"
	"time"
)

// Store holds in-memory watcher status reports. The source of truth for policy
// is always the k8s API (GPUHost CRDs); this is purely for observability.
type Store struct {
	mu      sync.RWMutex
	reports map[string]StatusReport // keyed by hostname
}

func NewStore() *Store {
	return &Store{reports: make(map[string]StatusReport)}
}

func (s *Store) Set(r StatusReport) {
	r.ReportedAt = time.Now().UTC()
	s.mu.Lock()
	s.reports[r.Hostname] = r
	s.mu.Unlock()
}

func (s *Store) Get(hostname string) (StatusReport, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.reports[hostname]
	return r, ok
}

func (s *Store) All() []StatusReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StatusReport, 0, len(s.reports))
	for _, r := range s.reports {
		out = append(out, r)
	}
	return out
}
