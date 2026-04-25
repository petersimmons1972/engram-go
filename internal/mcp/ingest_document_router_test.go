package mcp

// Integration tests for the format-router path inside runStreamIngest.
// These tests exercise runExportFanout (the testable core) with stub
// collaborators — no PostgreSQL or Ollama dependency required.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// minimalClaudeAIExport returns a valid single-conversation Claude.ai export.
func minimalClaudeAIExport() string {
	return `[{
		"uuid": "abc",
		"name": "Test Conversation",
		"created_at": "2024-01-01T00:00:00.000000Z",
		"updated_at": "2024-01-01T01:00:00.000000Z",
		"chat_messages": [
			{
				"uuid": "msg1",
				"text": "Hello from Claude",
				"sender": "human",
				"created_at": "2024-01-01T00:00:00.000000Z"
			}
		]
	}]`
}

// minimalChatGPTExport returns a valid single-conversation ChatGPT export.
func minimalChatGPTExport() string {
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

// TestRunStreamIngest_ClaudeAIExport: a valid Claude.ai export body produces
// export-fanout mode with one memory stored per conversation.
func TestRunStreamIngest_ClaudeAIExport(t *testing.T) {
	eng := &stubEngine{}
	back := newStubDocBackend()
	deps := storeDocumentDeps{engine: eng, backend: back}

	result, err := runStreamIngestWithDeps(context.Background(), deps, "testproject", minimalClaudeAIExport(), 8*1024*1024, 50*1024*1024)
	require.NoError(t, err)
	require.NotNil(t, result)

	m := resultMap(t, result)
	require.Equal(t, "ok", m["status"])
	require.Equal(t, "export-fanout", m["mode"])
	require.Equal(t, "claudeai", m["format"])

	count := int(m["memories_stored"].(float64))
	require.Equal(t, 1, count, "one conversation → one memory")

	ids, ok := m["memory_ids"].([]interface{})
	require.True(t, ok)
	require.Len(t, ids, 1)

	// Verify the memory was actually stored with the correct project.
	require.Len(t, eng.calls, 1)
	require.Equal(t, "testproject", eng.calls[0].mem.Project)
}

// TestRunStreamIngest_ChatGPTExport: a valid ChatGPT export body produces
// export-fanout mode with one memory stored per conversation.
func TestRunStreamIngest_ChatGPTExport(t *testing.T) {
	eng := &stubEngine{}
	back := newStubDocBackend()
	deps := storeDocumentDeps{engine: eng, backend: back}

	result, err := runStreamIngestWithDeps(context.Background(), deps, "testproject", minimalChatGPTExport(), 8*1024*1024, 50*1024*1024)
	require.NoError(t, err)
	require.NotNil(t, result)

	m := resultMap(t, result)
	require.Equal(t, "ok", m["status"])
	require.Equal(t, "export-fanout", m["mode"])
	require.Equal(t, "chatgpt", m["format"])

	count := int(m["memories_stored"].(float64))
	require.Equal(t, 1, count, "one conversation → one memory")

	ids, ok := m["memory_ids"].([]interface{})
	require.True(t, ok)
	require.Len(t, ids, 1)

	require.Len(t, eng.calls, 1)
	require.Equal(t, "testproject", eng.calls[0].mem.Project)
}

// TestRunStreamIngest_NonExportBody: plain markdown falls through to the
// existing single-memory path (mode must not be "export-fanout").
func TestRunStreamIngest_NonExportBody(t *testing.T) {
	eng := &stubEngine{}
	back := newStubDocBackend()
	deps := storeDocumentDeps{engine: eng, backend: back}

	body := "# My Notes\n\nThis is just a markdown document, not an export."
	result, err := runStreamIngestWithDeps(context.Background(), deps, "testproject", body, 8*1024*1024, 50*1024*1024)
	require.NoError(t, err)
	require.NotNil(t, result)

	m := resultMap(t, result)
	// Must use the standard single-memory path.
	require.NotEqual(t, "export-fanout", m["mode"], "plain markdown must not enter export-fanout mode")
	// One memory stored through the normal execStoreDocument path.
	require.Len(t, eng.calls, 1)
}

// TestRunStreamIngest_MalformedExportBody: body that triggers ClaudeAI
// detection but has a bad message timestamp must return an error and store
// nothing.
func TestRunStreamIngest_MalformedExportBody(t *testing.T) {
	eng := &stubEngine{}
	back := newStubDocBackend()
	deps := storeDocumentDeps{engine: eng, backend: back}

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

	result, err := runStreamIngestWithDeps(context.Background(), deps, "testproject", body, 8*1024*1024, 50*1024*1024)
	require.Error(t, err, "malformed export body must return an error")
	require.Nil(t, result)
	require.Empty(t, eng.calls, "no memories must be stored when parse fails")
}
