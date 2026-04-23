package claudeai_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/ingest/claudeai"
	"github.com/petersimmons1972/engram/internal/types"
)

// minimalJSON builds a one-conversation JSON array from the given chat_messages block.
func convJSON(chatMessages string) string {
	return `[{"uuid":"conv-1","name":"Test Conv","created_at":"2026-04-01T12:00:00.000Z","updated_at":"2026-04-01T12:30:00.000Z","chat_messages":` + chatMessages + `}]`
}

// TestParse_HappyPath: one conversation with two messages → one memory, correct fields.
func TestParse_HappyPath(t *testing.T) {
	input := `[
  {
    "uuid": "conv-uuid-1",
    "name": "Capital Questions",
    "created_at": "2026-04-01T12:00:00.000Z",
    "updated_at": "2026-04-01T12:30:00.000Z",
    "chat_messages": [
      {
        "uuid": "msg-1",
        "text": "What is the capital of France?",
        "sender": "human",
        "created_at": "2026-04-01T12:00:00.000Z"
      },
      {
        "uuid": "msg-2",
        "text": "The capital of France is Paris.",
        "sender": "assistant",
        "created_at": "2026-04-01T12:00:05.000Z"
      }
    ]
  }
]`

	memories, err := claudeai.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]

	// ID must be non-empty and differ from the conversation uuid.
	if m.ID == "" {
		t.Error("memory ID is empty")
	}
	if m.ID == "conv-uuid-1" {
		t.Error("memory ID must not reuse conversation uuid")
	}

	// Content must start with the conversation name as a heading.
	if !strings.HasPrefix(m.Content, "# Capital Questions") {
		t.Errorf("content does not start with expected heading; got: %q", m.Content[:min(80, len(m.Content))])
	}

	// Both messages must appear in the content.
	if !strings.Contains(m.Content, "What is the capital of France?") {
		t.Error("content missing human message text")
	}
	if !strings.Contains(m.Content, "The capital of France is Paris.") {
		t.Error("content missing assistant message text")
	}

	// Sender labels must appear as bold markdown.
	if !strings.Contains(m.Content, "**Human**") {
		t.Error("content missing **Human** label")
	}
	if !strings.Contains(m.Content, "**Assistant**") {
		t.Error("content missing **Assistant** label")
	}

	// MemoryType and Importance.
	if m.MemoryType != types.MemoryTypeContext {
		t.Errorf("expected MemoryType %q, got %q", types.MemoryTypeContext, m.MemoryType)
	}
	if m.Importance != 2 {
		t.Errorf("expected Importance 2, got %d", m.Importance)
	}

	// StorageMode.
	if m.StorageMode != "document" {
		t.Errorf("expected StorageMode 'document', got %q", m.StorageMode)
	}

	// Tags must include "claudeai" and "conversation".
	tagSet := make(map[string]bool)
	for _, tag := range m.Tags {
		tagSet[tag] = true
	}
	if !tagSet["claudeai"] {
		t.Error("missing tag 'claudeai'")
	}
	if !tagSet["conversation"] {
		t.Error("missing tag 'conversation'")
	}

	// CreatedAt should parse to 2026-04-01T12:00:00Z.
	wantCreated := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if !m.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt: expected %v, got %v", wantCreated, m.CreatedAt)
	}

	// UpdatedAt should parse to 2026-04-01T12:30:00Z.
	wantUpdated := time.Date(2026, 4, 1, 12, 30, 0, 0, time.UTC)
	if !m.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt: expected %v, got %v", wantUpdated, m.UpdatedAt)
	}

	// LastAccessed should be very recent (within the last minute).
	if time.Since(m.LastAccessed) > time.Minute {
		t.Errorf("LastAccessed %v appears stale", m.LastAccessed)
	}
}

// TestParse_EmptyArray: [] → empty slice, no error.
func TestParse_EmptyArray(t *testing.T) {
	memories, err := claudeai.Parse(strings.NewReader("[]"))
	if err != nil {
		t.Fatalf("unexpected error on empty array: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
}

// TestParse_EmptyConversation: conversation with no chat_messages → skipped.
func TestParse_EmptyConversation(t *testing.T) {
	input := `[
  {
    "uuid": "conv-empty",
    "name": "Abandoned",
    "created_at": "2026-04-01T10:00:00.000Z",
    "updated_at": "2026-04-01T10:00:00.000Z",
    "chat_messages": []
  }
]`
	memories, err := claudeai.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories for empty conversation, got %d", len(memories))
	}
}

// TestParse_MultipleConversations: 3 conversations → 3 memories.
func TestParse_MultipleConversations(t *testing.T) {
	input := `[
  {
    "uuid": "conv-1",
    "name": "First",
    "created_at": "2026-04-01T10:00:00.000Z",
    "updated_at": "2026-04-01T10:05:00.000Z",
    "chat_messages": [{"uuid":"m1","text":"hello","sender":"human","created_at":"2026-04-01T10:00:00.000Z"}]
  },
  {
    "uuid": "conv-2",
    "name": "Second",
    "created_at": "2026-04-02T10:00:00.000Z",
    "updated_at": "2026-04-02T10:05:00.000Z",
    "chat_messages": [{"uuid":"m2","text":"world","sender":"assistant","created_at":"2026-04-02T10:00:00.000Z"}]
  },
  {
    "uuid": "conv-3",
    "name": "Third",
    "created_at": "2026-04-03T10:00:00.000Z",
    "updated_at": "2026-04-03T10:05:00.000Z",
    "chat_messages": [{"uuid":"m3","text":"foo","sender":"human","created_at":"2026-04-03T10:00:00.000Z"}]
  }
]`
	memories, err := claudeai.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 3 {
		t.Errorf("expected 3 memories, got %d", len(memories))
	}

	// Each memory should contain its conversation's name.
	names := []string{"First", "Second", "Third"}
	for i, m := range memories {
		if !strings.Contains(m.Content, names[i]) {
			t.Errorf("memory[%d] does not contain conversation name %q", i, names[i])
		}
	}
}

// TestParse_InvalidJSON: broken JSON → error returned.
func TestParse_InvalidJSON(t *testing.T) {
	_, err := claudeai.Parse(strings.NewReader("{broken"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestParse_MissingRequiredField: conversation missing created_at → error.
func TestParse_MissingRequiredField(t *testing.T) {
	// created_at omitted entirely — must produce a clear error, not a zero-time memory.
	input := `[
  {
    "uuid": "conv-bad",
    "name": "No Timestamp",
    "updated_at": "2026-04-01T10:00:00.000Z",
    "chat_messages": [{"uuid":"m1","text":"hi","sender":"human","created_at":"2026-04-01T10:00:00.000Z"}]
  }
]`
	_, err := claudeai.Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing created_at, got nil")
	}
	if !strings.Contains(err.Error(), "created_at") {
		t.Errorf("error message should mention 'created_at', got: %v", err)
	}
}

// TestParse_EmptyMessageText: message with empty text → included in threaded output.
func TestParse_EmptyMessageText(t *testing.T) {
	input := convJSON(`[
    {"uuid":"m1","text":"","sender":"human","created_at":"2026-04-01T12:00:00.000Z"}
  ]`)
	memories, err := claudeai.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	// The sender label and timestamp must still appear.
	if !strings.Contains(memories[0].Content, "**Human**") {
		t.Error("content missing **Human** label for empty-text message")
	}
}

// TestParseFile_NotFound: nonexistent path → error wrapping os.ErrNotExist.
func TestParseFile_NotFound(t *testing.T) {
	_, err := claudeai.ParseFile("/nonexistent/path/conversations.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected error wrapping os.ErrNotExist, got: %v", err)
	}
}

// min is a local helper to avoid importing slices just for clamping in tests.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
