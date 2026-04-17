package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
	"golang.org/x/sync/errgroup"
)

const (
	reducerSystem = "You are a synthesis reducer. Multiple partial answers about the same question have been independently generated from memory shards. " +
		"Combine them into a single coherent answer, eliminating redundancy and resolving contradictions. " +
		"Preserve all cited memory IDs from the partial answers."

	maxReducerPartialBytes = 4096
)

// FanOutReason shards memories into groups of shardSize and runs ReasonOverMemories
// per shard concurrently (limited to maxWorkers), then reduces partial answers
// with a single Complete call. If len(memories) <= shardSize, calls
// ReasonOverMemories directly.
func FanOutReason(ctx context.Context, c *Client, question string, memories []*types.Memory, maxWorkers, shardSize int) (string, error) {
	if shardSize < 1 {
		shardSize = 15
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if len(memories) <= shardSize {
		return c.ReasonOverMemories(ctx, question, memories)
	}

	// Build shards.
	var shards [][]*types.Memory
	for i := 0; i < len(memories); i += shardSize {
		end := i + shardSize
		if end > len(memories) {
			end = len(memories)
		}
		shards = append(shards, memories[i:end])
	}

	partials := make([]string, len(shards))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxWorkers)

	for idx, shard := range shards {
		idx, shard := idx, shard
		g.Go(func() error {
			ans, err := c.ReasonOverMemories(gctx, question, shard)
			if err != nil {
				return fmt.Errorf("shard %d: %w", idx, err)
			}
			partials[idx] = ans
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}

	// Reducer prompt — cap combined partials at maxReducerPartialBytes.
	var sb strings.Builder
	fmt.Fprintf(&sb, "Question: %s\n\nPartial answers:\n", question)
	remaining := maxReducerPartialBytes
	for i, p := range partials {
		if remaining <= 0 {
			break
		}
		content := p
		if len(content) > remaining {
			content = content[:remaining]
		}
		fmt.Fprintf(&sb, "[%d] %s\n\n", i+1, content)
		remaining -= len(content)
	}

	reduced, err := c.Complete(ctx, reducerSystem, sb.String(), "claude-sonnet-4-6", "claude-opus-4-6", 2, 2048)
	if err != nil {
		return "", fmt.Errorf("reducer: %w", err)
	}
	return reduced, nil
}
