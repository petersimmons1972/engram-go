package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

type entry struct {
	CacheKey string            `json:"cache_key"`
	StoredAt time.Time         `json:"stored_at"`
	Result   types.ModelResult `json:"result"`
}

type store struct {
	Entries map[string]entry `json:"entries"` // keyed by model name
}

// Cache is a sha256-keyed result cache backed by a JSON file.
type Cache struct {
	path string
	mu   sync.Mutex
}

// New returns a Cache backed by the file at path.
func New(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) load() (store, error) {
	var s store
	s.Entries = map[string]entry{}
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func (c *Cache) save(s store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomic replacement.
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("cache write: %w", err)
	}
	return os.Rename(tmp, c.path)
}

// Write stores a result for model with the given cache key.
func (c *Cache) Write(model, cacheKey string, result types.ModelResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.load()
	if err != nil {
		return err
	}
	s.Entries[model] = entry{
		CacheKey: cacheKey,
		StoredAt: time.Now(),
		Result:   result,
	}
	return c.save(s)
}

// Read returns the cached result if the key matches and the entry is fresher than maxAge.
func (c *Cache) Read(model, cacheKey string, maxAge time.Duration) (types.ModelResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.load()
	if err != nil {
		return types.ModelResult{}, false, err
	}
	e, ok := s.Entries[model]
	if !ok {
		return types.ModelResult{}, false, nil
	}
	if e.CacheKey != cacheKey {
		return types.ModelResult{}, false, nil
	}
	if time.Since(e.StoredAt) > maxAge {
		return types.ModelResult{}, false, nil
	}
	return e.Result, true, nil
}
