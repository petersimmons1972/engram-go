// Package router detects the format of an AI conversation export (Claude.ai or
// ChatGPT) from the first few kilobytes of its content and dispatches to the
// appropriate parser to produce engram Memory records.
package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/petersimmons1972/engram/internal/ingest/chatgpt"
	"github.com/petersimmons1972/engram/internal/ingest/claudeai"
	"github.com/petersimmons1972/engram/internal/types"
)

// Format identifies the detected export format.
type Format string

const (
	FormatUnknown  Format = "unknown"
	FormatClaudeAI Format = "claudeai"
	FormatChatGPT  Format = "chatgpt"
)

// peekSize is the number of bytes we read when sniffing a stream. 4 KiB is
// enough to capture the first JSON object's keys without buffering the whole
// export.
const peekSize = 4096

// Detect inspects the first few KB of a byte slice and returns the detected
// export format. It does NOT consume or copy the full input.
//
// Detection rules:
//   - Must start with '[' (possibly after whitespace) — both formats are top-level JSON arrays.
//   - If the first object has a "chat_messages" key → ClaudeAI.
//   - If the first object has a "mapping" key → ChatGPT.
//   - Otherwise → Unknown.
func Detect(peek []byte) Format {
	// Trim leading whitespace to find the first non-space byte.
	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 {
		return FormatUnknown
	}

	// Both export formats are top-level JSON arrays.
	if trimmed[0] != '[' {
		return FormatUnknown
	}

	// Extract just enough JSON to attempt decoding the first object.
	// We decode an array of json.RawMessage so we can inspect the first element
	// cheaply without requiring the whole input to be valid.
	var firstElement json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(trimmed))

	// Consume the '[' token.
	tok, err := dec.Token()
	if err != nil || fmt.Sprintf("%v", tok) != "[" {
		return FormatUnknown
	}

	// Decode the first array element.
	if !dec.More() {
		return FormatUnknown
	}
	if err := dec.Decode(&firstElement); err != nil {
		return FormatUnknown
	}

	// Unmarshal into a generic map to inspect keys.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(firstElement, &obj); err != nil {
		return FormatUnknown
	}

	if _, ok := obj["chat_messages"]; ok {
		return FormatClaudeAI
	}
	if _, ok := obj["mapping"]; ok {
		return FormatChatGPT
	}
	return FormatUnknown
}

// ParseAuto reads r, detects the format, and dispatches to the matching
// parser. Returns (FormatUnknown, nil, nil) when no parser matches — the caller
// can then decide whether to fall back to single-document storage.
//
// Streams are fully buffered in memory (the stream ingest tool already buffers
// uploads; reusing that buffer is acceptable).
func ParseAuto(r io.Reader) (Format, []*types.Memory, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return FormatUnknown, nil, fmt.Errorf("router.ParseAuto: read: %w", err)
	}

	// Use up to peekSize bytes for detection.
	peek := data
	if len(peek) > peekSize {
		peek = data[:peekSize]
	}

	format := Detect(peek)
	switch format {
	case FormatUnknown:
		return FormatUnknown, nil, nil

	case FormatClaudeAI:
		memories, err := claudeai.Parse(strings.NewReader(string(data)))
		if err != nil {
			return FormatClaudeAI, nil, fmt.Errorf("router.ParseAuto: claudeai: %w", err)
		}
		return FormatClaudeAI, memories, nil

	case FormatChatGPT:
		memories, err := chatgpt.Parse(strings.NewReader(string(data)))
		if err != nil {
			return FormatChatGPT, nil, fmt.Errorf("router.ParseAuto: chatgpt: %w", err)
		}
		return FormatChatGPT, memories, nil

	default:
		// Defensive: new Format values not handled above.
		return FormatUnknown, nil, nil
	}
}
