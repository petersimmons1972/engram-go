package review

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Fingerprint string `json:"fingerprint"`
	Source      string `json:"source"`
}

type Sink interface {
	Record(ctx context.Context, e Event) error
}

type localSink struct {
	mu   sync.Mutex
	path string
}

func (s *localSink) Record(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(e)
}

type githubSink struct {
	baseURL string
	token   string
	repo    string
	client  *http.Client
}

func (s *githubSink) Record(ctx context.Context, e Event) error {
	body := map[string]any{
		"title":  e.Title,
		"body":   e.Body,
		"labels": []string{"bug", "db-degradation", "auto-generated"},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.baseURL, "/")+"/repos/"+s.repo+"/issues", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github issues create: %s", resp.Status)
	}
	return nil
}

type dedupeSink struct {
	mu   sync.Mutex
	seen map[string]struct{}
	next Sink
}

func (s *dedupeSink) Record(ctx context.Context, e Event) error {
	s.mu.Lock()
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	if _, ok := s.seen[e.Fingerprint]; ok {
		s.mu.Unlock()
		return nil
	}
	s.seen[e.Fingerprint] = struct{}{}
	s.mu.Unlock()
	return s.next.Record(ctx, e)
}

var defaultSink Sink = &dedupeSink{next: buildSink()}

func buildSink() Sink {
	if repo := os.Getenv("ENGRAM_GITHUB_REPOSITORY"); repo != "" && os.Getenv("GITHUB_TOKEN") != "" {
		baseURL := os.Getenv("ENGRAM_GITHUB_API_URL")
		if baseURL == "" {
			baseURL = "https://api.github.com"
		}
		return &githubSink{baseURL: baseURL, token: os.Getenv("GITHUB_TOKEN"), repo: repo, client: &http.Client{Timeout: 10 * time.Second}}
	}
	path := os.Getenv("ENGRAM_REVIEW_DRAFT_PATH")
	if path == "" {
		path = filepath.Join(os.TempDir(), "engram-review-drafts.jsonl")
	}
	return &localSink{path: path}
}

func Fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func RecordDBFailure(ctx context.Context, title, body, source string) {
	_ = defaultSink.Record(ctx, Event{
		Title:       title,
		Body:        body,
		Source:      source,
		Fingerprint: Fingerprint(title, source, body),
	})
}
