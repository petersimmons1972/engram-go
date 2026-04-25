package router_test

// TDD test suite for the format router. Tests are written before implementation
// so each can be run independently in the red state.

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/ingest/router"
	"github.com/stretchr/testify/require"
)

// ── Detect ────────────────────────────────────────────────────────────────────

func TestDetect_ClaudeAI(t *testing.T) {
	input := []byte(`[{"chat_messages": []}]`)
	require.Equal(t, router.FormatClaudeAI, router.Detect(input))
}

func TestDetect_ChatGPT(t *testing.T) {
	input := []byte(`[{"mapping": {}, "current_node": "x"}]`)
	require.Equal(t, router.FormatChatGPT, router.Detect(input))
}

func TestDetect_Unknown(t *testing.T) {
	input := []byte(`[{"foo": 1}]`)
	require.Equal(t, router.FormatUnknown, router.Detect(input))
}

func TestDetect_NotJSON(t *testing.T) {
	input := []byte(`hello world`)
	require.Equal(t, router.FormatUnknown, router.Detect(input))
}

func TestDetect_Empty(t *testing.T) {
	input := []byte(``)
	require.Equal(t, router.FormatUnknown, router.Detect(input))
}

func TestDetect_OnlyWhitespace(t *testing.T) {
	input := []byte("   \n  ")
	require.Equal(t, router.FormatUnknown, router.Detect(input))
}

// TestDetect_ObjectNotArray: both Claude.ai and ChatGPT exports are top-level
// arrays. A bare object must not be detected as either format.
func TestDetect_ObjectNotArray(t *testing.T) {
	input := []byte(`{"chat_messages": []}`)
	require.Equal(t, router.FormatUnknown, router.Detect(input))
}

// ── ParseAuto ─────────────────────────────────────────────────────────────────

// minimalClaudeAI returns a valid single-conversation Claude.ai export.
func minimalClaudeAI() string {
	return `[{
		"uuid": "abc",
		"name": "Test Conversation",
		"created_at": "2024-01-01T00:00:00.000000Z",
		"updated_at": "2024-01-01T01:00:00.000000Z",
		"chat_messages": [
			{
				"uuid": "msg1",
				"text": "Hello",
				"sender": "human",
				"created_at": "2024-01-01T00:00:00.000000Z"
			}
		]
	}]`
}

// minimalChatGPT returns a valid single-conversation ChatGPT export.
func minimalChatGPT() string {
	return `[{
		"title": "Test Chat",
		"create_time": 1704067200.0,
		"update_time": 1704070800.0,
		"current_node": "node-leaf",
		"mapping": {
			"node-root": {
				"id": "node-root",
				"message": null,
				"parent": null,
				"children": ["node-leaf"]
			},
			"node-leaf": {
				"id": "node-leaf",
				"message": {
					"id": "node-leaf",
					"author": {"role": "user"},
					"create_time": 1704067200.0,
					"content": {
						"content_type": "text",
						"parts": ["Hello from ChatGPT"]
					}
				},
				"parent": "node-root",
				"children": []
			}
		}
	}]`
}

func TestParseAuto_ClaudeAI_HappyPath(t *testing.T) {
	r := strings.NewReader(minimalClaudeAI())
	format, memories, err := router.ParseAuto(r)

	require.NoError(t, err)
	require.Equal(t, router.FormatClaudeAI, format)
	require.NotEmpty(t, memories)
	require.Contains(t, memories[0].Tags, "claudeai",
		"memories from Claude.ai export must carry the 'claudeai' tag")
}

func TestParseAuto_ChatGPT_HappyPath(t *testing.T) {
	r := strings.NewReader(minimalChatGPT())
	format, memories, err := router.ParseAuto(r)

	require.NoError(t, err)
	require.Equal(t, router.FormatChatGPT, format)
	require.NotEmpty(t, memories)
	require.Contains(t, memories[0].Tags, "chatgpt",
		"memories from ChatGPT export must carry the 'chatgpt' tag")
}

func TestParseAuto_UnknownFormat(t *testing.T) {
	r := strings.NewReader(`[{"random": "data"}]`)
	format, memories, err := router.ParseAuto(r)

	require.NoError(t, err)
	require.Equal(t, router.FormatUnknown, format)
	require.Nil(t, memories, "unknown format must return nil memories, not an empty slice")
}

// TestParseAuto_DetectMismatch: body looks like Claude.ai (has "chat_messages")
// but the individual message created_at is malformed so Parse will fail.
func TestParseAuto_DetectMismatch(t *testing.T) {
	body := `[{
		"uuid": "abc",
		"name": "Bad",
		"created_at": "2024-01-01T00:00:00.000000Z",
		"updated_at": "2024-01-01T01:00:00.000000Z",
		"chat_messages": [
			{
				"uuid": "m1",
				"text": "Hi",
				"sender": "human",
				"created_at": "NOT-A-DATE"
			}
		]
	}]`
	r := strings.NewReader(body)
	format, memories, err := router.ParseAuto(r)

	// Detect should succeed (returns FormatClaudeAI), but Parse should fail.
	require.Equal(t, router.FormatClaudeAI, format)
	require.Nil(t, memories)
	require.Error(t, err, "a malformed body on a detected format must return an error")
}
