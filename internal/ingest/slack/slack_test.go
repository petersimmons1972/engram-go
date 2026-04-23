// Package slack_test provides TDD tests for the Slack workspace export parser.
// Tests use archive/zip to build in-memory fixture zips — no real Slack exports needed.
package slack_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/ingest/slack"
	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

// zipBuilder assembles an in-memory zip archive. Call add() for each file,
// then bytes() to get the raw zip bytes.
type zipBuilder struct {
	buf bytes.Buffer
	w   *zip.Writer
}

func newZipBuilder() *zipBuilder {
	zb := &zipBuilder{}
	zb.w = zip.NewWriter(&zb.buf)
	return zb
}

func (zb *zipBuilder) add(name, content string) {
	f, err := zb.w.Create(name)
	if err != nil {
		panic(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		panic(err)
	}
}

func (zb *zipBuilder) finish() []byte {
	if err := zb.w.Close(); err != nil {
		panic(err)
	}
	return zb.buf.Bytes()
}

// parseBytes is a convenience wrapper: build a zip, call Parse with a bytes.Reader.
func parseBytes(data []byte) ([]*types.Memory, error) {
	r := bytes.NewReader(data)
	return slack.Parse(r, int64(len(data)))
}

// ---------------------------------------------------------------------------
// TestParse_HappyPath
// 1 channel ("general"), 2 messages from 2 users → 1 memory with resolved
// @real_name mentions, correct timestamps, three tags.
// ---------------------------------------------------------------------------

func TestParse_HappyPath(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[
		{"id":"U001","name":"jane","real_name":"Jane Doe"},
		{"id":"U002","name":"alex","real_name":"Alex Smith"}
	]`)
	zb.add("channels.json", `[
		{"id":"C001","name":"general"}
	]`)
	// Two messages from different users, ts seconds apart.
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"Hey team!","ts":"1711929600.000000"},
		{"type":"message","user":"U002","text":"Morning all.","ts":"1711929900.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0]

	// Content heading.
	if !strings.HasPrefix(m.Content, "# #general") {
		t.Errorf("content should start with '# #general', got: %q", firstN(m.Content, 80))
	}

	// Both message bodies present.
	if !strings.Contains(m.Content, "Hey team!") {
		t.Error("content missing first message text")
	}
	if !strings.Contains(m.Content, "Morning all.") {
		t.Error("content missing second message text")
	}

	// Sender labels use real_name.
	if !strings.Contains(m.Content, "**@Jane Doe**") {
		t.Error("content missing bold @Jane Doe label")
	}
	if !strings.Contains(m.Content, "**@Alex Smith**") {
		t.Error("content missing bold @Alex Smith label")
	}

	// MemoryType, Importance, StorageMode.
	if m.MemoryType != types.MemoryTypeContext {
		t.Errorf("expected MemoryType %q, got %q", types.MemoryTypeContext, m.MemoryType)
	}
	if m.Importance != 2 {
		t.Errorf("expected Importance 2, got %d", m.Importance)
	}
	if m.StorageMode != "document" {
		t.Errorf("expected StorageMode 'document', got %q", m.StorageMode)
	}

	// Tags: "slack", "channel", "general".
	tagSet := tagMap(m.Tags)
	if !tagSet["slack"] {
		t.Error("missing tag 'slack'")
	}
	if !tagSet["channel"] {
		t.Error("missing tag 'channel'")
	}
	if !tagSet["general"] {
		t.Error("missing tag 'general'")
	}
	if len(m.Tags) != 3 {
		t.Errorf("expected exactly 3 tags, got %d: %v", len(m.Tags), m.Tags)
	}

	// CreatedAt = ts of first message (1711929600).
	wantCreated := time.Unix(1711929600, 0).UTC()
	if !m.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt: expected %v, got %v", wantCreated, m.CreatedAt)
	}

	// UpdatedAt = ts of last message (1711929900).
	wantUpdated := time.Unix(1711929900, 0).UTC()
	if !m.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt: expected %v, got %v", wantUpdated, m.UpdatedAt)
	}

	// LastAccessed must be recent.
	if time.Since(m.LastAccessed) > time.Minute {
		t.Errorf("LastAccessed %v appears stale", m.LastAccessed)
	}
}

// ---------------------------------------------------------------------------
// TestParse_EmptyZip
// zip with users.json and channels.json but no channel dirs → empty slice.
// ---------------------------------------------------------------------------

func TestParse_EmptyZip(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[]`)
	zb.add("channels.json", `[]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
}

// ---------------------------------------------------------------------------
// TestParse_ChannelWithNoMessages
// channel dir exists, date file is empty array → channel skipped.
// ---------------------------------------------------------------------------

func TestParse_ChannelWithNoMessages(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[]`)
	zb.add("channels.json", `[{"id":"C001","name":"empty-chan"}]`)
	zb.add("empty-chan/2024-04-01.json", `[]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories for empty channel, got %d", len(memories))
	}
}

// ---------------------------------------------------------------------------
// TestParse_MultipleChannels
// "general" and "random" → 2 memories in alphabetical order.
// ---------------------------------------------------------------------------

func TestParse_MultipleChannels(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"alice","real_name":"Alice"}]`)
	zb.add("channels.json", `[
		{"id":"C001","name":"random"},
		{"id":"C002","name":"general"}
	]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"In general.","ts":"1711929600.000000"}
	]`)
	zb.add("random/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"In random.","ts":"1711929700.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}

	// Must be alphabetical: general < random.
	if !strings.HasPrefix(memories[0].Content, "# #general") {
		t.Errorf("first memory should be #general, got: %q", firstN(memories[0].Content, 40))
	}
	if !strings.HasPrefix(memories[1].Content, "# #random") {
		t.Errorf("second memory should be #random, got: %q", firstN(memories[1].Content, 40))
	}
}

// ---------------------------------------------------------------------------
// TestParse_ThreadReply
// parent message + reply (thread_ts != ts) → reply body prefixed "-> ".
// ---------------------------------------------------------------------------

func TestParse_ThreadReply(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"bob","real_name":"Bob"}]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"Parent post.","ts":"1711929600.000000","thread_ts":"1711929600.000000"},
		{"type":"message","user":"U001","text":"Reply here.","ts":"1711929700.000000","thread_ts":"1711929600.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	content := memories[0].Content

	// Parent message: no prefix.
	if !strings.Contains(content, "Parent post.") {
		t.Error("missing parent post text")
	}

	// Reply: prefixed with "-> ".
	if !strings.Contains(content, "-> Reply here.") {
		t.Errorf("reply should be prefixed with '-> ', content:\n%s", content)
	}

	// Parent post should NOT have the -> prefix.
	if strings.Contains(content, "-> Parent post.") {
		t.Error("parent post should not have the reply prefix")
	}
}

// ---------------------------------------------------------------------------
// TestParse_UserMentionResolved
// text "Hi <@U001>" with U001 real_name "Alice Smith" → "Hi @Alice Smith".
// ---------------------------------------------------------------------------

func TestParse_UserMentionResolved(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"alice","real_name":"Alice Smith"}]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"Hi <@U001>!","ts":"1711929600.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	if !strings.Contains(memories[0].Content, "Hi @Alice Smith!") {
		t.Errorf("user mention not resolved: content=%q", memories[0].Content)
	}
	// Raw mention form must not appear.
	if strings.Contains(memories[0].Content, "<@U001>") {
		t.Error("raw mention <@U001> should have been resolved")
	}
}

// ---------------------------------------------------------------------------
// TestParse_UnknownUserMention
// text "Hi <@U_GHOST>" with no matching user → "Hi @unknown-user".
// ---------------------------------------------------------------------------

func TestParse_UnknownUserMention(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"Hi <@U_GHOST>!","ts":"1711929600.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	if !strings.Contains(memories[0].Content, "Hi @unknown-user!") {
		t.Errorf("unknown mention not resolved to @unknown-user: content=%q", memories[0].Content)
	}
}

// ---------------------------------------------------------------------------
// TestParse_NonMessageTypeSkipped
// type:"file_share" message skipped; type:"message" in same channel included.
// ---------------------------------------------------------------------------

func TestParse_NonMessageTypeSkipped(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"carol","real_name":"Carol"}]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"file_share","user":"U001","text":"Shared a file.","ts":"1711929600.000000"},
		{"type":"message","user":"U001","text":"Real message.","ts":"1711929700.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	content := memories[0].Content
	if strings.Contains(content, "Shared a file.") {
		t.Error("file_share message should not appear in content")
	}
	if !strings.Contains(content, "Real message.") {
		t.Error("type:message should appear in content")
	}
}

// ---------------------------------------------------------------------------
// TestParse_MessageMissingUserSkipped
// message with no "user" field → skipped; channel still included if others exist.
// ---------------------------------------------------------------------------

func TestParse_MessageMissingUserSkipped(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"dave","real_name":"Dave"}]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","text":"Bot says hi.","ts":"1711929600.000000"},
		{"type":"message","user":"U001","text":"Human says hi.","ts":"1711929700.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory (channel has one valid message), got %d", len(memories))
	}

	content := memories[0].Content
	if strings.Contains(content, "Bot says hi.") {
		t.Error("message without user field should be skipped")
	}
	if !strings.Contains(content, "Human says hi.") {
		t.Error("message with user field should be included")
	}
}

// ---------------------------------------------------------------------------
// TestParse_MessageOrderedByTimestamp
// Two date files with interleaved ts → messages sorted ascending across both.
// ---------------------------------------------------------------------------

func TestParse_MessageOrderedByTimestamp(t *testing.T) {
	zb := newZipBuilder()
	zb.add("users.json", `[{"id":"U001","name":"eve","real_name":"Eve"}]`)
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	// File for April 2 has an EARLIER ts than file for April 1 — intentionally
	// inverted to confirm ordering is by ts, not by filename.
	zb.add("general/2024-04-02.json", `[
		{"type":"message","user":"U001","text":"Earliest msg (file 2).","ts":"1711929600.000000"}
	]`)
	zb.add("general/2024-04-01.json", `[
		{"type":"message","user":"U001","text":"Latest msg (file 1).","ts":"1711929900.000000"}
	]`)

	memories, err := parseBytes(zb.finish())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	content := memories[0].Content
	posEarliest := strings.Index(content, "Earliest msg")
	posLatest := strings.Index(content, "Latest msg")
	if posEarliest == -1 || posLatest == -1 {
		t.Fatal("expected both messages in content")
	}
	if posEarliest > posLatest {
		t.Error("messages are not in ascending timestamp order")
	}
}

// ---------------------------------------------------------------------------
// TestParse_InvalidZip
// not a zip at all → returns error.
// ---------------------------------------------------------------------------

func TestParse_InvalidZip(t *testing.T) {
	garbage := []byte("this is not a zip file")
	_, err := parseBytes(garbage)
	if err == nil {
		t.Fatal("expected error for invalid zip, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestParse_MissingUsersJSON
// zip with channels but no users.json → error with clear message.
// ---------------------------------------------------------------------------

func TestParse_MissingUsersJSON(t *testing.T) {
	zb := newZipBuilder()
	zb.add("channels.json", `[{"id":"C001","name":"general"}]`)
	// No users.json.

	_, err := parseBytes(zb.finish())
	if err == nil {
		t.Fatal("expected error for missing users.json, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "users.json") {
		t.Errorf("error should mention 'users.json', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestParseFile_NotFound
// nonexistent path → error wrapping os.ErrNotExist.
// ---------------------------------------------------------------------------

func TestParseFile_NotFound(t *testing.T) {
	_, err := slack.ParseFile("/nonexistent/path/export.zip")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected error wrapping os.ErrNotExist, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func tagMap(tags []string) map[string]bool {
	m := make(map[string]bool, len(tags))
	for _, t := range tags {
		m[t] = true
	}
	return m
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
