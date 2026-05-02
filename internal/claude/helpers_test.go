package claude_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

func makeClient(t *testing.T, baseURL string) *claude.Client {
	t.Helper()
	c := claude.NewWithTransport("test-key", nil)
	require.NotNil(t, c)
	c.BaseURL = baseURL
	return c
}
