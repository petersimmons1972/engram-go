// Package chatgpt parses ChatGPT conversations.json export files into engram
// Memory records. One non-empty conversation (with at least one non-system
// message on the current_node path) produces one Memory.
//
// ChatGPT exports use a tree structure (mapping of node-id → node) rather than
// a flat array. The "current_node" field identifies the leaf of the branch the
// user kept; we walk parent links from that leaf back to the root, reverse the
// path, then render the messages in chronological order.
package chatgpt

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// JSON shape types
// ---------------------------------------------------------------------------

// conversation is one element in the top-level conversations.json array.
type conversation struct {
	Title       string             `json:"title"`
	CreateTime  *float64           `json:"create_time"`
	UpdateTime  *float64           `json:"update_time"`
	Mapping     map[string]mapNode `json:"mapping"`
	CurrentNode string             `json:"current_node"`
}

// mapNode is one entry in the "mapping" map.
type mapNode struct {
	ID       string   `json:"id"`
	Message  *message `json:"message"`
	Parent   *string  `json:"parent"`
	Children []string `json:"children"`
}

// message is the optional message payload inside a mapNode.
type message struct {
	ID         string   `json:"id"`
	Author     author   `json:"author"`
	CreateTime *float64 `json:"create_time"`
	Content    content  `json:"content"`
}

// author describes who produced the message.
type author struct {
	Role string `json:"role"`
}

// content holds the message body. Parts is []json.RawMessage so we can detect
// whether each element is a JSON string or an object.
type content struct {
	ContentType string            `json:"content_type"`
	Parts       []json.RawMessage `json:"parts"`
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Parse reads a ChatGPT conversations.json byte stream and returns one
// types.Memory per conversation that has at least one non-system message on
// the current_node path. Messages are assembled into a single threaded Content
// string in the same style as the claudeai package.
func Parse(r io.Reader) ([]*types.Memory, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("chatgpt.Parse: read: %w", err)
	}

	var convs []conversation
	if err := json.Unmarshal(data, &convs); err != nil {
		return nil, fmt.Errorf("chatgpt.Parse: unmarshal: %w", err)
	}

	var memories []*types.Memory
	for i, conv := range convs {
		// Both conversation-level timestamps are required — a zero-time memory
		// is meaningless and mirrors the behaviour of the claudeai parser.
		if conv.CreateTime == nil {
			return nil, fmt.Errorf("chatgpt.Parse: conversation[%d] title=%q: missing create_time", i, conv.Title)
		}
		if conv.UpdateTime == nil {
			return nil, fmt.Errorf("chatgpt.Parse: conversation[%d] title=%q: missing update_time", i, conv.Title)
		}

		createdAt := epochToTime(*conv.CreateTime)
		updatedAt := epochToTime(*conv.UpdateTime)

		// Walk the tree to get the ordered path of nodes for current_node.
		path := pathToRoot(conv)

		// Build the content string; skip conversations with no renderable messages.
		contentStr, hasMessages, err := buildContent(conv.Title, path, createdAt)
		if err != nil {
			return nil, fmt.Errorf("chatgpt.Parse: conversation[%d] title=%q: %w", i, conv.Title, err)
		}
		if !hasMessages {
			continue
		}

		m := &types.Memory{
			ID:           types.NewMemoryID(),
			Content:      contentStr,
			MemoryType:   types.MemoryTypeContext,
			Importance:   2,
			StorageMode:  "document",
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
			LastAccessed: time.Now().UTC(),
			Tags:         []string{"chatgpt", "conversation"},
		}
		memories = append(memories, m)
	}

	if memories == nil {
		memories = []*types.Memory{}
	}
	return memories, nil
}

// ParseFile is a convenience wrapper that opens the file and calls Parse.
func ParseFile(path string) ([]*types.Memory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("chatgpt.ParseFile: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// ---------------------------------------------------------------------------
// Tree traversal
// ---------------------------------------------------------------------------

// pathToRoot walks from current_node up through parent links to reconstruct
// the conversation path in chronological (root-first) order.
//
// If current_node is empty or absent, we fall back to a forward walk from the
// root following first children — this handles malformed exports.
func pathToRoot(conv conversation) []mapNode {
	if conv.Mapping == nil {
		return nil
	}

	start := conv.CurrentNode
	if start == "" {
		// Fallback: find the root (node with no parent) and walk forward.
		start = findRoot(conv.Mapping)
	}

	// Walk parent links to collect the path in reverse (leaf → root).
	var reversed []mapNode
	visited := make(map[string]bool)
	cur := start
	for {
		node, ok := conv.Mapping[cur]
		if !ok || visited[cur] {
			break
		}
		visited[cur] = true
		reversed = append(reversed, node)
		if node.Parent == nil {
			break
		}
		cur = *node.Parent
	}

	// Reverse to get root → leaf order.
	path := make([]mapNode, len(reversed))
	for i, n := range reversed {
		path[len(reversed)-1-i] = n
	}
	return path
}

// findRoot returns the ID of the node with no parent (the tree root).
// If multiple nodes lack a parent it returns the first one found.
func findRoot(mapping map[string]mapNode) string {
	for id, node := range mapping {
		if node.Parent == nil {
			return id
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Content assembly
// ---------------------------------------------------------------------------

// buildContent assembles the threaded markdown body for one conversation.
// It returns the content string, a boolean indicating whether at least one
// non-system message was rendered, and any error.
func buildContent(title string, path []mapNode, convCreateTime time.Time) (string, bool, error) {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(strings.TrimSpace(title))
	sb.WriteString("\n")

	hasMessages := false
	for _, node := range path {
		if node.Message == nil {
			// Root or internal structural node — skip.
			continue
		}
		msg := node.Message

		// Skip system messages entirely.
		if strings.ToLower(msg.Author.Role) == "system" {
			continue
		}

		hasMessages = true
		sb.WriteString("\n")

		// Resolve timestamp: prefer message create_time, fall back to conversation.
		var ts time.Time
		if msg.CreateTime != nil {
			ts = epochToTime(*msg.CreateTime)
		} else {
			ts = convCreateTime
		}

		label := roleLabel(msg.Author.Role)
		fmt.Fprintf(&sb, "**%s** (%s):\n", label, ts.UTC().Format(time.RFC3339))

		body := assembleBody(msg.Content)
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	return sb.String(), hasMessages, nil
}

// assembleBody joins the parts of a content block into a single string.
// For non-text content_types we still attempt to use parts if they are strings;
// any part that is not a JSON string literal is replaced with a sentinel.
func assembleBody(c content) string {
	if len(c.Parts) == 0 {
		return ""
	}

	var segments []string
	for _, raw := range c.Parts {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			// Part is not a JSON string (it's an object, number, etc.).
			segments = append(segments, "[non-text content omitted]")
		} else {
			segments = append(segments, s)
		}
	}
	return strings.Join(segments, "\n\n")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// epochToTime converts a Unix epoch float (seconds + fractional) to a UTC
// time.Time. ChatGPT exports encode timestamps as float64, e.g. 1712000000.123.
func epochToTime(epoch float64) time.Time {
	sec := math.Floor(epoch)
	frac := epoch - sec
	ns := int64(math.Round(frac * 1e9))
	return time.Unix(int64(sec), ns).UTC()
}

// roleLabel maps ChatGPT author roles to display-friendly labels.
// "user" → "Human", "assistant" → "Assistant", "tool" → "Tool",
// anything else is title-cased.
func roleLabel(role string) string {
	switch strings.ToLower(role) {
	case "user":
		return "Human"
	case "assistant":
		return "Assistant"
	case "tool":
		return "Tool"
	default:
		if len(role) == 0 {
			return "Unknown"
		}
		return strings.ToUpper(role[:1]) + role[1:]
	}
}
