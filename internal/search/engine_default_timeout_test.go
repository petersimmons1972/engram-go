package search

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/stretchr/testify/require"
)

type timeoutClient struct {
	dims int
}

func (t *timeoutClient) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, t.dims), nil
}
func (t *timeoutClient) EmbedWithModel(_ context.Context, text string) ([]float32, string, error) {
	vec, err := t.Embed(context.Background(), text)
	return vec, t.Name(), err
}
func (t *timeoutClient) Name() string    { return "timeout-client" }
func (t *timeoutClient) Dimensions() int { return t.dims }

var _ embed.Client = (*timeoutClient)(nil)

// TestEmbedRecallTimeoutDefaultIs500 verifies that a production-default engine,
// when no timeout env var is set, uses a 500ms embed timeout for recall.
func TestEmbedRecallTimeoutDefaultIs500(t *testing.T) {
	ctx := context.Background()
	t.Setenv("ENGRAM_EMBED_RECALL_TIMEOUT_MS", "")
	project := testutil.UniqueProject("embed-timeout-default")
	backend, err := db.NewPostgresBackend(ctx, project, testutil.DSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	eng := New(
		ctx,
		backend,
		&timeoutClient{dims: 1024},
		project,
		"http://ollama:11434", "llama3.2", false, nil, 0,
	)
	t.Cleanup(func() { eng.Close() })

	require.Equal(t, 500*time.Millisecond, eng.embedRecallTimeout)
}
