package chatgpt_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/ingest/chatgpt"
	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// TestParse_HappyPath: 1 conversation, 2-message linear path → 1 memory.
// ---------------------------------------------------------------------------

func TestParse_HappyPath(t *testing.T) {
	input := `[
  {
    "title": "Capital Questions",
    "create_time": 1712000000.123,
    "update_time": 1712000300.456,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-1"]
      },
      "node-1": {
        "id": "node-1",
        "message": {
          "id": "msg-1",
          "author": {"role": "user", "name": null},
          "create_time": 1712000000.0,
          "content": {"content_type": "text", "parts": ["What is the capital of France?"]}
        },
        "parent": "root",
        "children": ["node-2"]
      },
      "node-2": {
        "id": "node-2",
        "message": {
          "id": "msg-2",
          "author": {"role": "assistant", "name": null},
          "create_time": 1712000005.789,
          "content": {"content_type": "text", "parts": ["The capital of France is Paris."]}
        },
        "parent": "node-1",
        "children": []
      }
    },
    "current_node": "node-2"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]

	if m.ID == "" {
		t.Error("memory ID is empty")
	}

	// Content must start with the conversation title as a heading.
	if !strings.HasPrefix(m.Content, "# Capital Questions") {
		t.Errorf("content does not start with expected heading; got: %q", m.Content[:minInt(80, len(m.Content))])
	}

	// Both messages must appear in the content.
	if !strings.Contains(m.Content, "What is the capital of France?") {
		t.Error("content missing user message text")
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

	// Timestamps must appear in RFC3339 format (UTC).
	if !strings.Contains(m.Content, "2024-04-01T") {
		t.Error("content missing RFC3339 timestamp")
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

	// Tags must include "chatgpt" and "conversation".
	tagSet := make(map[string]bool)
	for _, tag := range m.Tags {
		tagSet[tag] = true
	}
	if !tagSet["chatgpt"] {
		t.Error("missing tag 'chatgpt'")
	}
	if !tagSet["conversation"] {
		t.Error("missing tag 'conversation'")
	}

	// CreatedAt parsed from 1712000000.123.
	// Use a tolerance of 1ms because float64 epoch arithmetic is imprecise at
	// the nanosecond level; the parser uses math.Round internally.
	wantCreated := time.Unix(1712000000, 123_000_000).UTC()
	if diff := m.CreatedAt.Sub(wantCreated); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("CreatedAt: expected ~%v, got %v (diff %v)", wantCreated, m.CreatedAt, diff)
	}

	// UpdatedAt parsed from 1712000300.456.
	wantUpdated := time.Unix(1712000300, 456_000_000).UTC()
	if diff := m.UpdatedAt.Sub(wantUpdated); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("UpdatedAt: expected ~%v, got %v (diff %v)", wantUpdated, m.UpdatedAt, diff)
	}

	// LastAccessed must be recent.
	if time.Since(m.LastAccessed) > time.Minute {
		t.Errorf("LastAccessed %v appears stale", m.LastAccessed)
	}
}

// ---------------------------------------------------------------------------
// TestParse_EmptyArray: [] → empty slice, no error.
// ---------------------------------------------------------------------------

func TestParse_EmptyArray(t *testing.T) {
	memories, err := chatgpt.Parse(strings.NewReader("[]"))
	if err != nil {
		t.Fatalf("unexpected error on empty array: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
}

// ---------------------------------------------------------------------------
// TestParse_NoMessages: conversation with only a root-null-message node → skipped.
// ---------------------------------------------------------------------------

func TestParse_NoMessages(t *testing.T) {
	input := `[
  {
    "title": "Empty Conv",
    "create_time": 1712000000.0,
    "update_time": 1712000000.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": []
      }
    },
    "current_node": "root"
  }
]`
	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories for no-message conversation, got %d", len(memories))
	}
}

// ---------------------------------------------------------------------------
// TestParse_SystemMessagesFilteredOut: system messages must not appear in content.
// ---------------------------------------------------------------------------

func TestParse_SystemMessagesFilteredOut(t *testing.T) {
	input := `[
  {
    "title": "Filtered Conv",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-sys"]
      },
      "node-sys": {
        "id": "node-sys",
        "message": {
          "id": "msg-sys",
          "author": {"role": "system"},
          "create_time": 1712000000.0,
          "content": {"content_type": "text", "parts": ["You are a helpful assistant."]}
        },
        "parent": "root",
        "children": ["node-user"]
      },
      "node-user": {
        "id": "node-user",
        "message": {
          "id": "msg-user",
          "author": {"role": "user"},
          "create_time": 1712000010.0,
          "content": {"content_type": "text", "parts": ["Hello!"]}
        },
        "parent": "node-sys",
        "children": ["node-asst"]
      },
      "node-asst": {
        "id": "node-asst",
        "message": {
          "id": "msg-asst",
          "author": {"role": "assistant"},
          "create_time": 1712000020.0,
          "content": {"content_type": "text", "parts": ["Hi there!"]}
        },
        "parent": "node-user",
        "children": []
      }
    },
    "current_node": "node-asst"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory (system-filtered still yields memory), got %d", len(memories))
	}

	content := memories[0].Content
	if strings.Contains(content, "You are a helpful assistant") {
		t.Error("system message text must not appear in content")
	}
	if strings.Contains(content, "**System**") {
		t.Error("System label must not appear in content")
	}
	if !strings.Contains(content, "**Human**") {
		t.Error("content missing **Human** label")
	}
	if !strings.Contains(content, "**Assistant**") {
		t.Error("content missing **Assistant** label")
	}
}

// ---------------------------------------------------------------------------
// TestParse_BranchedTree: current_node determines path; other branch excluded.
// ---------------------------------------------------------------------------

func TestParse_BranchedTree(t *testing.T) {
	// node-1 has two children: node-2a (selected via current_node) and node-2b.
	input := `[
  {
    "title": "Branched",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-1"]
      },
      "node-1": {
        "id": "node-1",
        "message": {
          "id": "msg-1",
          "author": {"role": "user"},
          "create_time": 1712000000.0,
          "content": {"content_type": "text", "parts": ["Question?"]}
        },
        "parent": "root",
        "children": ["node-2a", "node-2b"]
      },
      "node-2a": {
        "id": "node-2a",
        "message": {
          "id": "msg-2a",
          "author": {"role": "assistant"},
          "create_time": 1712000010.0,
          "content": {"content_type": "text", "parts": ["Answer A — the selected branch."]}
        },
        "parent": "node-1",
        "children": []
      },
      "node-2b": {
        "id": "node-2b",
        "message": {
          "id": "msg-2b",
          "author": {"role": "assistant"},
          "create_time": 1712000010.0,
          "content": {"content_type": "text", "parts": ["Answer B — the discarded branch."]}
        },
        "parent": "node-1",
        "children": []
      }
    },
    "current_node": "node-2a"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	content := memories[0].Content
	if !strings.Contains(content, "Answer A — the selected branch.") {
		t.Error("content missing selected branch answer")
	}
	if strings.Contains(content, "Answer B — the discarded branch.") {
		t.Error("content must not include discarded branch")
	}
}

// ---------------------------------------------------------------------------
// TestParse_NullMessageTimestamp: message with null create_time falls back to
// conversation create_time.
// ---------------------------------------------------------------------------

func TestParse_NullMessageTimestamp(t *testing.T) {
	input := `[
  {
    "title": "Null TS Conv",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-1"]
      },
      "node-1": {
        "id": "node-1",
        "message": {
          "id": "msg-1",
          "author": {"role": "user"},
          "create_time": null,
          "content": {"content_type": "text", "parts": ["Hello with null timestamp."]}
        },
        "parent": "root",
        "children": []
      }
    },
    "current_node": "node-1"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	content := memories[0].Content
	// The fallback timestamp comes from conversation create_time (1712000000.0 → 2024-04-01T...).
	if !strings.Contains(content, "**Human**") {
		t.Error("content missing **Human** label")
	}
	// A timestamp from the conversation must be present.
	if !strings.Contains(content, "2024-04-01T") {
		t.Error("content missing fallback timestamp from conversation create_time")
	}
}

// ---------------------------------------------------------------------------
// TestParse_NonTextContent: code content_type with string parts → joined.
// Non-string parts → "[non-text content omitted]".
// ---------------------------------------------------------------------------

func TestParse_NonTextContent(t *testing.T) {
	// String parts in a "code" content_type → should join them.
	input := `[
  {
    "title": "Code Message",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-1"]
      },
      "node-1": {
        "id": "node-1",
        "message": {
          "id": "msg-1",
          "author": {"role": "assistant"},
          "create_time": 1712000000.0,
          "content": {"content_type": "code", "parts": ["print('hello')", "print('world')"]}
        },
        "parent": "root",
        "children": []
      }
    },
    "current_node": "node-1"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error for code content: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	if !strings.Contains(memories[0].Content, "print('hello')") {
		t.Error("content missing string parts from code content_type")
	}

	// Non-string parts: parts is an array containing an object, not a string.
	inputObj := `[
  {
    "title": "Object Parts",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {
        "id": "root",
        "message": null,
        "parent": null,
        "children": ["node-1"]
      },
      "node-1": {
        "id": "node-1",
        "message": {
          "id": "msg-1",
          "author": {"role": "user"},
          "create_time": 1712000000.0,
          "content": {"content_type": "multimodal_text", "parts": [{"image_url": "http://example.com/img.png"}]}
        },
        "parent": "root",
        "children": []
      }
    },
    "current_node": "node-1"
  }
]`

	memories2, err := chatgpt.Parse(strings.NewReader(inputObj))
	if err != nil {
		t.Fatalf("unexpected error for object parts: %v", err)
	}
	if len(memories2) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories2))
	}
	if !strings.Contains(memories2[0].Content, "[non-text content omitted]") {
		t.Errorf("expected '[non-text content omitted]' for object parts, got: %q",
			memories2[0].Content)
	}
}

// ---------------------------------------------------------------------------
// TestParse_MultipleConversations: 2 conversations → 2 memories in order.
// ---------------------------------------------------------------------------

func TestParse_MultipleConversations(t *testing.T) {
	input := `[
  {
    "title": "First",
    "create_time": 1712000000.0,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {"id":"root","message":null,"parent":null,"children":["n1"]},
      "n1": {
        "id":"n1",
        "message":{"id":"m1","author":{"role":"user"},"create_time":1712000000.0,
                   "content":{"content_type":"text","parts":["First question"]}},
        "parent":"root","children":[]
      }
    },
    "current_node": "n1"
  },
  {
    "title": "Second",
    "create_time": 1712001000.0,
    "update_time": 1712001100.0,
    "mapping": {
      "root": {"id":"root","message":null,"parent":null,"children":["n1"]},
      "n1": {
        "id":"n1",
        "message":{"id":"m1","author":{"role":"assistant"},"create_time":1712001000.0,
                   "content":{"content_type":"text","parts":["Second answer"]}},
        "parent":"root","children":[]
      }
    },
    "current_node": "n1"
  }
]`

	memories, err := chatgpt.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}
	if !strings.Contains(memories[0].Content, "First") {
		t.Error("first memory missing 'First' title")
	}
	if !strings.Contains(memories[1].Content, "Second") {
		t.Error("second memory missing 'Second' title")
	}
}

// ---------------------------------------------------------------------------
// TestParse_InvalidJSON: broken JSON → error.
// ---------------------------------------------------------------------------

func TestParse_InvalidJSON(t *testing.T) {
	_, err := chatgpt.Parse(strings.NewReader("{broken"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestParseFile_NotFound: nonexistent path → error wrapping os.ErrNotExist.
// ---------------------------------------------------------------------------

func TestParseFile_NotFound(t *testing.T) {
	_, err := chatgpt.ParseFile("/nonexistent/path/conversations.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected error wrapping os.ErrNotExist, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestParse_NullConversationTimestamps: null create_time or update_time → error.
// ---------------------------------------------------------------------------

func TestParse_NullConversationTimestamps(t *testing.T) {
	// null create_time.
	inputNullCreate := `[
  {
    "title": "Null Create",
    "create_time": null,
    "update_time": 1712000100.0,
    "mapping": {
      "root": {"id":"root","message":null,"parent":null,"children":["n1"]},
      "n1": {
        "id":"n1",
        "message":{"id":"m1","author":{"role":"user"},"create_time":1712000000.0,
                   "content":{"content_type":"text","parts":["hello"]}},
        "parent":"root","children":[]
      }
    },
    "current_node": "n1"
  }
]`
	_, err := chatgpt.Parse(strings.NewReader(inputNullCreate))
	if err == nil {
		t.Fatal("expected error for null create_time, got nil")
	}
	if !strings.Contains(err.Error(), "create_time") {
		t.Errorf("error should mention 'create_time', got: %v", err)
	}

	// null update_time.
	inputNullUpdate := `[
  {
    "title": "Null Update",
    "create_time": 1712000000.0,
    "update_time": null,
    "mapping": {
      "root": {"id":"root","message":null,"parent":null,"children":["n1"]},
      "n1": {
        "id":"n1",
        "message":{"id":"m1","author":{"role":"user"},"create_time":1712000000.0,
                   "content":{"content_type":"text","parts":["hello"]}},
        "parent":"root","children":[]
      }
    },
    "current_node": "n1"
  }
]`
	_, err = chatgpt.Parse(strings.NewReader(inputNullUpdate))
	if err == nil {
		t.Fatal("expected error for null update_time, got nil")
	}
	if !strings.Contains(err.Error(), "update_time") {
		t.Errorf("error should mention 'update_time', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
