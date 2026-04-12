package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

const (
	maxMemoriesInReason      = 20
	maxMemoryContentInReason = 1000
)

const reasonSystem = "You are a reasoning engine operating over a structured memory store. " +
	"Reason over the provided memories to answer the question. Cite memory IDs where relevant. " +
	"IMPORTANT: If conflicts or contradicting claims exist among the memories, you MUST explicitly " +
	"name the rejected alternatives and state why they were rejected. " +
	"Format rejected claims as: 'Note: [rejected claim from memory ID] is not correct because [reason]. " +
	"The authoritative source is [memory ID].' " +
	"If uncertain about which claim is authoritative, state both and flag the uncertainty. " +
	"If uncertain about the answer entirely, escalate to your advisor."

// ReasonSystemPrompt returns the system prompt used by all reasoning operations.
// Exposed for testing so the constant cannot silently regress.
func ReasonSystemPrompt() string {
	return reasonSystem
}

// ReasonOverMemories recalls and synthesizes an answer from memories.
// It limits the memory set to maxMemoriesInReason (20) entries, truncates each
// memory's Content to maxMemoryContentInReason (1000) characters, and asks
// Claude to produce a free-form answer citing memory IDs where relevant.
func (c *Client) ReasonOverMemories(ctx context.Context, question string, memories []*types.Memory) (string, error) {
	// Cap the memory list to avoid oversized prompts.
	if len(memories) > maxMemoriesInReason {
		memories = memories[:maxMemoriesInReason]
	}

	prompt := buildReasonPrompt(question, memories)

	result, err := c.Complete(ctx, reasonSystem, prompt, "claude-sonnet-4-6", "claude-opus-4-6", 2, 2048)
	if err != nil {
		return "", err
	}

	return result, nil
}

// ReasonWithConflictAwareness synthesizes an answer from an EvidenceMap, using a
// conflict-aware prompt so Claude acknowledges uncertainty when contradictions exist.
func (c *Client) ReasonWithConflictAwareness(ctx context.Context, question string, ev EvidenceMap) (string, error) {
	memories := ev.Memories
	if len(memories) > maxMemoriesInReason {
		ev.Memories = memories[:maxMemoriesInReason]
	}
	prompt := BuildConflictAwarePrompt(question, ev)
	return c.Complete(ctx, reasonSystem, prompt, "claude-sonnet-4-6", "claude-opus-4-6", 2, 2048)
}

// buildReasonPrompt constructs the numbered memory listing prompt.
func buildReasonPrompt(question string, memories []*types.Memory) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Question: %s\n\nMemories:\n", question)

	for i, m := range memories {
		content := m.Content
		if len(content) > maxMemoryContentInReason {
			content = content[:maxMemoryContentInReason]
		}
		fmt.Fprintf(&sb, "[%d] ID: %s\n%s\n\n", i+1, m.ID, content)
	}

	return strings.TrimRight(sb.String(), "\n")
}
