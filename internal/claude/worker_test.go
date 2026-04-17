package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func makeMemories(n int) []*types.Memory {
	out := make([]*types.Memory, n)
	for i := 0; i < n; i++ {
		out[i] = &types.Memory{
			ID:      strings.Repeat("a", 31) + string(rune('0'+i%10)),
			Content: "mem content",
		}
	}
	return out
}

func TestFanOutReason_SmallCorpus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "answer for small corpus"}},
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	answer, err := claude.FanOutReason(context.Background(), c, "q?", makeMemories(10), 8, 15)
	require.NoError(t, err)
	require.Contains(t, answer, "answer")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls), "small corpus should trigger 1 call")
}

func TestFanOutReason_LargeCorpus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "partial answer"}},
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	answer, err := claude.FanOutReason(context.Background(), c, "q?", makeMemories(30), 8, 15)
	require.NoError(t, err)
	require.NotEmpty(t, answer)
	// 30 memories → 2 shards + 1 reducer = 3 calls minimum.
	require.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(3))
}
