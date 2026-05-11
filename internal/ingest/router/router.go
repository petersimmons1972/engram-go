// Package router detects the format of an AI conversation export (Claude.ai or
// ChatGPT) from the first few kilobytes of its content and dispatches to the
// appropriate parser to produce engram Memory records.
package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/petersimmons1972/engram/internal/ingest/chatgpt"
	"github.com/petersimmons1972/engram/internal/ingest/claudeai"
	"github.com/petersimmons1972/engram/internal/types"
)

const DefaultParseMaxBytes = 50 * 1024 * 1024

// Format identifies the detected export format.
type Format string

const (
	FormatUnknown  Format = "unknown"
	FormatClaudeAI Format = "claudeai"
	FormatChatGPT  Format = "chatgpt"
	FormatSlack    Format = "slack"
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
	return ParseAutoWithLimit(r, DefaultParseMaxBytes)
}

func ParseAutoWithLimit(r io.Reader, maxBytes int64) (Format, []*types.Memory, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultParseMaxBytes
	}
	lr := &io.LimitedReader{R: r, N: maxBytes + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return FormatUnknown, nil, fmt.Errorf("router.ParseAutoWithLimit: read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return FormatUnknown, nil, fmt.Errorf("router.ParseAutoWithLimit: input exceeds maximum size (%d bytes)", maxBytes)
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
			return FormatClaudeAI, nil, fmt.Errorf("router.ParseAutoWithLimit: claudeai: %w", err)
		}
		return FormatClaudeAI, memories, nil

	case FormatChatGPT:
		memories, err := chatgpt.Parse(strings.NewReader(string(data)))
		if err != nil {
			return FormatChatGPT, nil, fmt.Errorf("router.ParseAutoWithLimit: chatgpt: %w", err)
		}
		return FormatChatGPT, memories, nil

	default:
		// Defensive: new Format values not handled above.
		return FormatUnknown, nil, nil
	}
}

var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// DetectFromPath opens the file at path, reads the first peekSize bytes, and
// returns the detected format. ZIP files (magic bytes PK\x03\x04) are
// classified as FormatSlack; all others fall back to Detect.
func DetectFromPath(path string) Format {
	f, err := os.Open(path)
	if err != nil {
		return FormatUnknown
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, peekSize)
	n, _ := f.Read(buf)
	if n < 4 {
		return FormatUnknown
	}
	peek := buf[:n]
	if bytes.HasPrefix(peek, zipMagic) {
		return FormatSlack
	}
	return Detect(peek)
}
