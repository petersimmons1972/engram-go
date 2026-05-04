// Package claudeai parses Claude.ai conversations.json export files into
// engram Memory records. One non-empty conversation produces one Memory.
package claudeai

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// conversation is the JSON shape of one element in Claude.ai conversations.json.
type conversation struct {
	UUID         string        `json:"uuid"`
	Name         string        `json:"name"`
	CreatedAtRaw string        `json:"created_at"`
	UpdatedAtRaw string        `json:"updated_at"`
	Messages     []chatMessage `json:"chat_messages"`
}

// chatMessage is one turn inside a conversation.
type chatMessage struct {
	UUID         string `json:"uuid"`
	Text         string `json:"text"`
	Sender       string `json:"sender"`
	CreatedAtRaw string `json:"created_at"`
}

// Parse reads a Claude.ai conversations.json byte stream and returns one
// types.Memory per non-empty conversation. Messages within a conversation
// are joined into a single threaded Content string.
func Parse(r io.Reader) ([]*types.Memory, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("claudeai.Parse: read: %w", err)
	}

	var convs []conversation
	if err := json.Unmarshal(data, &convs); err != nil {
		return nil, fmt.Errorf("claudeai.Parse: unmarshal: %w", err)
	}

	var memories []*types.Memory
	for i, conv := range convs {
		// Skip conversations that were created but never had any messages.
		if len(conv.Messages) == 0 {
			continue
		}

		// created_at is required — a zero-time memory is meaningless.
		if conv.CreatedAtRaw == "" {
			return nil, fmt.Errorf("claudeai.Parse: conversation[%d] uuid=%q: missing created_at", i, conv.UUID)
		}
		createdAt, err := time.Parse(time.RFC3339Nano, conv.CreatedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("claudeai.Parse: conversation[%d] uuid=%q: invalid created_at %q: %w", i, conv.UUID, conv.CreatedAtRaw, err)
		}

		if conv.UpdatedAtRaw == "" {
			return nil, fmt.Errorf("claudeai.Parse: conversation[%d] uuid=%q: missing updated_at", i, conv.UUID)
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, conv.UpdatedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("claudeai.Parse: conversation[%d] uuid=%q: invalid updated_at %q: %w", i, conv.UUID, conv.UpdatedAtRaw, err)
		}

		content, err := buildContent(conv)
		if err != nil {
			return nil, fmt.Errorf("claudeai.Parse: conversation[%d] uuid=%q: %w", i, conv.UUID, err)
		}

		m := &types.Memory{
			ID:           types.NewMemoryID(),
			Content:      content,
			MemoryType:   types.MemoryTypeContext,
			Importance:   2,
			StorageMode:  "document",
			CreatedAt:    createdAt.UTC(),
			UpdatedAt:    updatedAt.UTC(),
			LastAccessed: time.Now().UTC(),
			Tags:         []string{"claudeai", "conversation"},
		}
		memories = append(memories, m)
	}

	if memories == nil {
		memories = []*types.Memory{}
	}
	return memories, nil
}

// buildContent assembles the threaded markdown body for one conversation.
func buildContent(conv conversation) (string, error) {
	var sb strings.Builder

	// Heading: the conversation name.
	name := strings.TrimSpace(conv.Name)
	sb.WriteString("# ")
	sb.WriteString(name)
	sb.WriteString("\n")

	for _, msg := range conv.Messages {
		sb.WriteString("\n")

		// Parse the message timestamp for the inline label.
		// A missing timestamp is not fatal for individual messages — we omit the
		// time component rather than reject the whole conversation. This matches
		// the observed reality that some attachment-only messages have no body.
		var ts string
		if msg.CreatedAtRaw != "" {
			t, err := time.Parse(time.RFC3339Nano, msg.CreatedAtRaw)
			if err != nil {
				return "", fmt.Errorf("message uuid=%q: invalid created_at %q: %w", msg.UUID, msg.CreatedAtRaw, err)
			}
			ts = t.UTC().Format(time.RFC3339)
		}

		// Sender label: "human" → "Human", "assistant" → "Assistant".
		label := senderLabel(msg.Sender)

		if ts != "" {
			fmt.Fprintf(&sb, "**%s** (%s):\n", label, ts)
		} else {
			fmt.Fprintf(&sb, "**%s**:\n", label)
		}

		text := strings.TrimSpace(msg.Text)
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// senderLabel converts the raw sender value to a display-friendly label.
// "human" → "Human", "assistant" → "Assistant", anything else is title-cased.
func senderLabel(sender string) string {
	switch strings.ToLower(sender) {
	case "human":
		return "Human"
	case "assistant":
		return "Assistant"
	default:
		if len(sender) == 0 {
			return "Unknown"
		}
		return strings.ToUpper(sender[:1]) + sender[1:]
	}
}

// ParseFile is a convenience wrapper that opens the file and calls Parse.
func ParseFile(path string) ([]*types.Memory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("claudeai.ParseFile: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}
