package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockSessionDB tracks calls to TouchSession to verify coalescing.
type mockSessionDB struct {
	mu             sync.Mutex
	touchCallCount int
	touchCalls     []time.Time
}

func (m *mockSessionDB) TouchSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.touchCallCount++
	m.touchCalls = append(m.touchCalls, time.Now())
	return nil
}

func (m *mockSessionDB) RegisterSession(ctx context.Context, sessionID string, apiKeyHash string) error {
	return nil
}

func (m *mockSessionDB) UnregisterSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockSessionDB) ListActiveSessions(ctx context.Context, within time.Duration, apiKeyHash string) ([]string, error) {
	return nil, nil
}

// TestTouchSessionCoalesces verifies that rapid TouchSession calls (within 30s)
// are coalesced into a single DB write. Without coalescing, each request spawns
// a goroutine that calls TouchSession, causing unbounded goroutine growth (#553).
func TestTouchSessionCoalesces(t *testing.T) {
	mockDB := &mockSessionDB{}
	server := &Server{
		pool:              nil,
		cfg:               Config{SessionDB: mockDB},
		uploads:           make(map[string]*uploadSession),
		embedDegraded:     &atomic.Bool{},
		sessionTouchTimes: make(map[string]time.Time),
		sessionFingerprints: sync.Map{},
	}

	sessionID := "test-session-123"
	apiKey := "test-api-key"

	// Store a fingerprint so withSessionFingerprint doesn't reject the session
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(sessionID))
	server.sessionFingerprints.Store(sessionID, mac.Sum(nil))

	// Wrap the handler to create requests
	handler := server.withSessionFingerprint(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		apiKey,
	)

	// Fire 5 rapid requests (all within 30s coalescing window)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/?sessionId="+sessionID, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d failed with status %d", i, w.Code)
		}
	}

	// Allow time for goroutines to complete
	time.Sleep(100 * time.Millisecond)

	// Verify TouchSession was called only once (coalesced), not 5 times
	mockDB.mu.Lock()
	callCount := mockDB.touchCallCount
	mockDB.mu.Unlock()

	if callCount != 1 {
		t.Errorf("expected 1 TouchSession call (coalesced), got %d", callCount)
	}
}

// TestTouchSessionBreaksCoalesceAfter30s verifies that after 30+ seconds,
// a new TouchSession DB call is made (coalescing window resets).
func TestTouchSessionBreaksCoalesceAfter30s(t *testing.T) {
	mockDB := &mockSessionDB{}
	server := &Server{
		pool:              nil,
		cfg:               Config{SessionDB: mockDB},
		uploads:           make(map[string]*uploadSession),
		embedDegraded:     &atomic.Bool{},
		sessionTouchTimes: make(map[string]time.Time),
		sessionFingerprints: sync.Map{},
	}

	sessionID := "test-session-456"
	apiKey := "test-api-key-2"

	// Store a fingerprint
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(sessionID))
	server.sessionFingerprints.Store(sessionID, mac.Sum(nil))

	handler := server.withSessionFingerprint(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		apiKey,
	)

	// First request triggers a touch
	req1 := httptest.NewRequest("POST", "/?sessionId="+sessionID, nil)
	req1.Header.Set("Authorization", "Bearer "+apiKey)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	time.Sleep(50 * time.Millisecond)

	// Manually advance the clock by setting the last touch time to 31 seconds ago
	server.sessionTouchMu.Lock()
	server.sessionTouchTimes[sessionID] = time.Now().Add(-31 * time.Second)
	server.sessionTouchMu.Unlock()

	// Second request after 31 seconds should trigger another touch
	req2 := httptest.NewRequest("POST", "/?sessionId="+sessionID, nil)
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	time.Sleep(50 * time.Millisecond)

	// Verify TouchSession was called twice (coalescing broke after 30s)
	mockDB.mu.Lock()
	callCount := mockDB.touchCallCount
	mockDB.mu.Unlock()

	if callCount != 2 {
		t.Errorf("expected 2 TouchSession calls after 30s, got %d", callCount)
	}
}
