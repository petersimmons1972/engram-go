// Package slack parses Slack workspace export .zip archives into engram Memory
// records. One non-empty channel produces one Memory containing all messages
// from that channel in ascending timestamp order.
//
// Slack export layout:
//
//	export.zip
//	├── users.json           array of user objects
//	├── channels.json        array of channel objects
//	└── <channel-name>/
//	    ├── YYYY-MM-DD.json  array of messages
//	    └── ...
package slack

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// JSON shape types
// ---------------------------------------------------------------------------

// slackUser is one element from users.json.
type slackUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name"`
}

// slackChannel is one element from channels.json.
type slackChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// slackMessage is one element from a YYYY-MM-DD.json channel file.
type slackMessage struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Text     string `json:"text"`
	Ts       string `json:"ts"`
	ThreadTs string `json:"thread_ts"`
}

// parsedMessage is an intermediate form after timestamp parsing.
type parsedMessage struct {
	user     string // resolved display name (e.g. "@Jane Doe")
	text     string // text with mentions resolved
	ts       time.Time
	isReply  bool // thread_ts present and != ts
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Parse reads a Slack workspace export .zip from r. r must be a seekable reader
// (io.ReaderAt) — use ParseFile for a file path, or wrap a []byte with
// bytes.NewReader for in-memory use. size is the byte length of the zip data.
//
// Returns one types.Memory per channel that has at least one non-skipped message.
// Memories are returned in alphabetical channel-name order.
func Parse(r io.ReaderAt, size int64) ([]*types.Memory, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("slack.Parse: open zip: %w", err)
	}

	// Build a lookup from zip entry name → *zip.File for O(1) access.
	fileMap := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		fileMap[f.Name] = f
	}

	// users.json is required.
	users, err := parseUsers(fileMap)
	if err != nil {
		return nil, fmt.Errorf("slack.Parse: %w", err)
	}

	// channels.json is optional — if absent we proceed with no channels.
	channels, err := parseChannels(fileMap)
	if err != nil {
		return nil, fmt.Errorf("slack.Parse: %w", err)
	}

	// Sort channel names so output is deterministic.
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Name < channels[j].Name
	})

	var memories []*types.Memory
	for _, ch := range channels {
		msgs, err := loadChannelMessages(fileMap, ch.Name)
		if err != nil {
			return nil, fmt.Errorf("slack.Parse: channel %q: %w", ch.Name, err)
		}
		if len(msgs) == 0 {
			continue
		}

		// Resolve user IDs to display names and mentions in text.
		resolved := resolveMessages(msgs, users)
		if len(resolved) == 0 {
			continue
		}

		m := buildMemory(ch.Name, resolved)
		memories = append(memories, m)
	}

	if memories == nil {
		memories = []*types.Memory{}
	}
	return memories, nil
}

// ParseFile is a convenience wrapper that opens the .zip file at path and calls Parse.
func ParseFile(path string) ([]*types.Memory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("slack.ParseFile: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("slack.ParseFile: stat: %w", err)
	}
	return Parse(f, info.Size())
}

// ---------------------------------------------------------------------------
// Zip file helpers
// ---------------------------------------------------------------------------

// readZipFile reads the full contents of a zip entry.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// parseUsers loads and decodes users.json. Returns error if the file is absent.
func parseUsers(fileMap map[string]*zip.File) (map[string]slackUser, error) {
	zf, ok := fileMap["users.json"]
	if !ok {
		return nil, fmt.Errorf("users.json not found in zip")
	}
	data, err := readZipFile(zf)
	if err != nil {
		return nil, fmt.Errorf("read users.json: %w", err)
	}
	var arr []slackUser
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("decode users.json: %w", err)
	}
	m := make(map[string]slackUser, len(arr))
	for _, u := range arr {
		m[u.ID] = u
	}
	return m, nil
}

// parseChannels loads and decodes channels.json. Returns an empty slice if the
// file is absent (channels are discovered from directory structure in that case).
func parseChannels(fileMap map[string]*zip.File) ([]slackChannel, error) {
	zf, ok := fileMap["channels.json"]
	if !ok {
		// No channels.json → no channels to process.
		return nil, nil
	}
	data, err := readZipFile(zf)
	if err != nil {
		return nil, fmt.Errorf("read channels.json: %w", err)
	}
	var arr []slackChannel
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("decode channels.json: %w", err)
	}
	return arr, nil
}

// loadChannelMessages finds all YYYY-MM-DD.json files under <channelName>/ in
// the zip and concatenates their message arrays. Only messages with type
// "message" and a non-empty "user" field are retained.
func loadChannelMessages(fileMap map[string]*zip.File, channelName string) ([]slackMessage, error) {
	prefix := channelName + "/"
	var raw []slackMessage

	for name, zf := range fileMap {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Only YYYY-MM-DD.json files immediately inside the channel dir.
		rest := strings.TrimPrefix(name, prefix)
		if strings.Contains(rest, "/") {
			// Nested subdirectory — skip.
			continue
		}
		if !strings.HasSuffix(rest, ".json") {
			continue
		}

		data, err := readZipFile(zf)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		var msgs []slackMessage
		if err := json.Unmarshal(data, &msgs); err != nil {
			return nil, fmt.Errorf("decode %q: %w", name, err)
		}
		for _, msg := range msgs {
			if msg.Type != "message" {
				continue
			}
			if msg.User == "" {
				continue
			}
			raw = append(raw, msg)
		}
	}
	return raw, nil
}

// ---------------------------------------------------------------------------
// Message resolution and assembly
// ---------------------------------------------------------------------------

// mentionRe matches Slack user mention tokens like <@U12345>.
var mentionRe = regexp.MustCompile(`<@([A-Z0-9_]+)>`)

// resolveMentions replaces <@UXXX> tokens in text with @<display_name>.
// Unknown user IDs become "@unknown-user".
func resolveMentions(text string, users map[string]slackUser) string {
	return mentionRe.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the user ID from inside <@ … >.
		uid := match[2 : len(match)-1]
		if u, ok := users[uid]; ok {
			name := u.RealName
			if name == "" {
				name = u.Name
			}
			return "@" + name
		}
		return "@unknown-user"
	})
}

// displayName returns the best available display name for a user ID.
func displayName(uid string, users map[string]slackUser) string {
	if u, ok := users[uid]; ok {
		if u.RealName != "" {
			return u.RealName
		}
		return u.Name
	}
	return uid
}

// resolveMessages converts raw slackMessages to parsedMessages. It resolves
// user IDs to display names, resolves in-text mentions, parses timestamps, and
// determines thread-reply status. Messages with unparseable timestamps are skipped.
func resolveMessages(raw []slackMessage, users map[string]slackUser) []parsedMessage {
	out := make([]parsedMessage, 0, len(raw))
	for _, msg := range raw {
		ts, err := parseSlackTs(msg.Ts)
		if err != nil {
			// Unparse-able timestamp — skip this message rather than crashing.
			continue
		}

		text := resolveMentions(msg.Text, users)
		name := displayName(msg.User, users)
		isReply := msg.ThreadTs != "" && msg.ThreadTs != msg.Ts

		out = append(out, parsedMessage{
			user:    "@" + name,
			text:    text,
			ts:      ts,
			isReply: isReply,
		})
	}

	// Sort by timestamp ascending — files may arrive in any order.
	sort.Slice(out, func(i, j int) bool {
		return out[i].ts.Before(out[j].ts)
	})
	return out
}

// parseSlackTs parses a Slack timestamp string (e.g. "1712000000.123456") into
// a time.Time. The format is seconds.microseconds as a decimal string.
func parseSlackTs(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, fmt.Errorf("empty ts")
	}
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse ts %q: %w", ts, err)
	}
	sec := math.Floor(f)
	frac := f - sec
	ns := int64(math.Round(frac * 1e9))
	return time.Unix(int64(sec), ns).UTC(), nil
}

// buildMemory assembles a types.Memory for one channel from its resolved messages.
func buildMemory(channelName string, msgs []parsedMessage) *types.Memory {
	var sb strings.Builder
	sb.WriteString("# #")
	sb.WriteString(channelName)
	sb.WriteString("\n")

	for _, msg := range msgs {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("**%s** (%s):\n", msg.user, msg.ts.Format(time.RFC3339)))

		body := msg.text
		if msg.isReply {
			body = "-> " + body
		}
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	createdAt := msgs[0].ts
	updatedAt := msgs[len(msgs)-1].ts

	return &types.Memory{
		ID:           types.NewMemoryID(),
		Content:      sb.String(),
		MemoryType:   types.MemoryTypeContext,
		Importance:   2,
		StorageMode:  "document",
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		LastAccessed: time.Now().UTC(),
		Tags:         []string{"slack", "channel", channelName},
	}
}
