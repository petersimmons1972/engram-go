package mcp

// Tests for input validation helpers added by the security sprint:
//   - validateProjectName (Fix #249: bidi/homoglyph rejection + NFC normalization)
//   - toStringSlice (Fix #252: control character rejection in tags)
//   - validateContent (Fix #253: C0/C1 control character rejection in content)

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── validateProjectName ──────────────────────────────────────────────────────

func TestValidateProjectName_AcceptsNormal(t *testing.T) {
	cases := []string{
		"default",
		"clearwatch",
		"my-project-123",
		"project with spaces",
		"日本語プロジェクト",
		"café",        // NFC normalizable
		"a",           // minimum length
		"project_v2",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			require.NoError(t, validateProjectName(c))
		})
	}
}

func TestValidateProjectName_RejectsEmpty(t *testing.T) {
	err := validateProjectName("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestValidateProjectName_RejectsWhitespaceOnly(t *testing.T) {
	err := validateProjectName("   ")
	require.Error(t, err)
}

func TestValidateProjectName_RejectsTooLong(t *testing.T) {
	long := make([]byte, maxProjectNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	err := validateProjectName(string(long))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max length")
}

func TestValidateProjectName_RejectsBidiControlChars(t *testing.T) {
	bidiCases := []struct {
		name    string
		codepoint rune
	}{
		{"zero-width space U+200B", 0x200B},
		{"ZWNJ U+200C", 0x200C},
		{"ZWJ U+200D", 0x200D},
		{"LRM U+200E", 0x200E},
		{"RLM U+200F", 0x200F},
		{"LRE U+202A", 0x202A},
		{"RLE U+202B", 0x202B},
		{"PDF U+202C", 0x202C},
		{"LRO U+202D", 0x202D},
		{"RLO U+202E", 0x202E},
		{"word joiner U+2060", 0x2060},
		{"FSI U+2068", 0x2068},
		{"PDI U+2069", 0x2069},
		{"BOM U+FEFF", 0xFEFF},
		{"Arabic letter mark U+061C", 0x061C},
	}
	for _, tc := range bidiCases {
		t.Run(tc.name, func(t *testing.T) {
			// Embed the codepoint in an otherwise valid project name.
			s := "proj" + string([]rune{tc.codepoint}) + "ect" //nolint:misspell
			err := validateProjectName(s)
			require.Error(t, err, "expected error for codepoint U+%04X", tc.codepoint)
			assert.Contains(t, err.Error(), "disallowed codepoint")
		})
	}
}

// ── toStringSlice ────────────────────────────────────────────────────────────

func TestToStringSlice_AcceptsNormal(t *testing.T) {
	input := []any{"go", "tdd", "engram", "tag with spaces"}
	got, err := toStringSlice(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "tdd", "engram", "tag with spaces"}, got)
}

func TestToStringSlice_AcceptsTabLFCR(t *testing.T) {
	// HT (0x09), LF (0x0A), CR (0x0D) are explicitly allowed.
	input := []any{"tag\twith\ttabs", "tag\nwith\nnewline", "tag\rwith\rCR"}
	got, err := toStringSlice(input)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestToStringSlice_RejectsNUL(t *testing.T) {
	input := []any{"valid", "bad\x00tag"}
	_, err := toStringSlice(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed control character")
}

func TestToStringSlice_RejectsC0ControlChars(t *testing.T) {
	controlCases := []byte{0x01, 0x07, 0x08, 0x0B, 0x0C, 0x0E, 0x1F}
	for _, b := range controlCases {
		t.Run(string(rune(b)), func(t *testing.T) {
			input := []any{"good", "bad" + string([]byte{b}) + "tag"}
			_, err := toStringSlice(input)
			require.Error(t, err, "expected error for byte 0x%02X", b)
		})
	}
}

func TestToStringSlice_RejectsDEL(t *testing.T) {
	input := []any{"tag\x7fwith-del"}
	_, err := toStringSlice(input)
	require.Error(t, err)
}

func TestToStringSlice_NilInput(t *testing.T) {
	got, err := toStringSlice(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestToStringSlice_RespectsMaxTagCount(t *testing.T) {
	var input []any
	for i := 0; i < maxTagCount+10; i++ {
		input = append(input, "tag")
	}
	got, err := toStringSlice(input)
	require.NoError(t, err)
	assert.Len(t, got, maxTagCount)
}

// ── validateContent ──────────────────────────────────────────────────────────

func TestValidateContent_AcceptsNormal(t *testing.T) {
	cases := []string{
		"Hello, world!",
		"Multi\nline\ncontent",
		"Tab\tseparated",
		"CR\rLF\ncombined",
		"Unicode: 日本語 café 🚀",
	}
	for _, c := range cases {
		t.Run(c[:min(len(c), 20)], func(t *testing.T) {
			require.NoError(t, validateContent(c))
		})
	}
}

func TestValidateContent_RejectsNUL(t *testing.T) {
	err := validateContent("content with \x00 null")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "U+0000")
}

func TestValidateContent_RejectsC0ControlChars(t *testing.T) {
	// 0x01-0x08, 0x0B, 0x0C, 0x0E-0x1F
	cases := []struct {
		name string
		b    byte
	}{
		{"SOH 0x01", 0x01},
		{"BEL 0x07", 0x07},
		{"BS 0x08", 0x08},
		{"VT 0x0B", 0x0B},
		{"FF 0x0C", 0x0C},
		{"SO 0x0E", 0x0E},
		{"US 0x1F", 0x1F},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateContent("bad" + string([]byte{tc.b}) + "content")
			require.Error(t, err, "expected error for byte 0x%02X", tc.b)
		})
	}
}

func TestValidateContent_RejectsDEL(t *testing.T) {
	err := validateContent("del\x7fchar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "U+007F")
}

func TestValidateContent_RejectsC1ControlChars(t *testing.T) {
	// U+0080 to U+009F as UTF-8 two-byte sequences
	for r := rune(0x80); r <= 0x9F; r++ {
		r := r
		t.Run("C1", func(t *testing.T) {
			err := validateContent("c1" + string(r))
			require.Error(t, err, "expected error for codepoint U+%04X", r)
		})
	}
}

func TestValidateContent_AllowsHTLFCR(t *testing.T) {
	require.NoError(t, validateContent("tab\there"))
	require.NoError(t, validateContent("newline\nhere"))
	require.NoError(t, validateContent("cr\rhere"))
}

func TestValidateContent_Empty(t *testing.T) {
	require.NoError(t, validateContent(""))
}

// min is a small helper for Go <1.21 compatibility in tests.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
