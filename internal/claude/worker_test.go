package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
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
		out[i] = &types.Memory{ID: strings.Repeat("a", 31) + string(rune('0'+i%10)), Content: "mem content"}
	}
	return out
}

func TestFanOutReason_SmallCorpus(t *testing.T) {
	var calls int32
	c := claude.NewWithTransport("test-key", reasonRT(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		payload, _ := json.Marshal(textResponse("answer for small corpus"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	answer, err := claude.FanOutReason(context.Background(), c, "q?", makeMemories(10), 8, 15)
	require.NoError(t, err)
	require.Contains(t, answer, "answer")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestFanOutReason_LargeCorpus(t *testing.T) {
	var calls int32
	c := claude.NewWithTransport("test-key", reasonRT(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		payload, _ := json.Marshal(textResponse("partial answer"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	answer, err := claude.FanOutReason(context.Background(), c, "q?", makeMemories(30), 8, 15)
	require.NoError(t, err)
	require.NotEmpty(t, answer)
	require.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(3))
}
