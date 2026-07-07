package mcp

// Unit tests for the typed-argument extraction helpers at the center of
// issue #1281 (MCP tools registered with empty input schemas → array/number
// params silently discarded when a schema-less client stringifies them).
//
// These tests exercise the helpers directly (no MCP transport involved) so
// the coercion/error-surfacing contract is pinned independently of any
// particular handler.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ── coerceToInt / coerceToFloat / coerceToBool ──────────────────────────────

func TestCoerceToFloat_NativeNumber(t *testing.T) {
	f, ok := coerceToFloat(5.0)
	require.True(t, ok)
	require.Equal(t, 5.0, f)
}

func TestCoerceToFloat_JSONEncodedString(t *testing.T) {
	// Defense-in-depth: a client that ignores the (now-declared) schema and
	// still stringifies a number must not silently lose the value.
	f, ok := coerceToFloat("250")
	require.True(t, ok)
	require.Equal(t, 250.0, f)
}

func TestCoerceToFloat_GarbageString(t *testing.T) {
	_, ok := coerceToFloat("not-a-number")
	require.False(t, ok)
}

func TestCoerceToFloat_WrongType(t *testing.T) {
	_, ok := coerceToFloat(true)
	require.False(t, ok)
}

func TestCoerceToInt_RoundsFloat(t *testing.T) {
	n, ok := coerceToInt(4.6)
	require.True(t, ok)
	require.Equal(t, 5, n)
}

func TestCoerceToInt_JSONEncodedString(t *testing.T) {
	n, ok := coerceToInt("5")
	require.True(t, ok)
	require.Equal(t, 5, n)
}

func TestCoerceToBool_Native(t *testing.T) {
	b, ok := coerceToBool(true)
	require.True(t, ok)
	require.True(t, b)
}

func TestCoerceToBool_StringFallback(t *testing.T) {
	b, ok := coerceToBool("true")
	require.True(t, ok)
	require.True(t, b)

	b, ok = coerceToBool("false")
	require.True(t, ok)
	require.False(t, b)
}

func TestCoerceToBool_GarbageString(t *testing.T) {
	_, ok := coerceToBool("yes")
	require.False(t, ok, "only exact true/false strings are coerced — no truthy guessing")
}

// ── getInt / getFloat / getBool: string-fallback coercion, still lenient ───

func TestGetInt_AcceptsStringifiedNumber(t *testing.T) {
	args := map[string]any{"limit": "250"}
	require.Equal(t, 250, getInt(args, "limit", 50))
}

func TestGetInt_FallsBackToDefaultOnGarbage(t *testing.T) {
	args := map[string]any{"limit": "not-a-number"}
	require.Equal(t, 50, getInt(args, "limit", 50))
}

func TestGetInt_AbsentKeyUsesDefault(t *testing.T) {
	require.Equal(t, 50, getInt(map[string]any{}, "limit", 50))
}

func TestGetBool_AcceptsStringifiedBool(t *testing.T) {
	args := map[string]any{"immutable": "true"}
	require.True(t, getBool(args, "immutable", false))
}

// ── requireOptionalInt: the loud-error path used by load-bearing params ────

func TestRequireOptionalInt_AbsentKey(t *testing.T) {
	n, present, err := requireOptionalInt(map[string]any{}, "importance")
	require.NoError(t, err)
	require.False(t, present)
	require.Equal(t, 0, n)
}

func TestRequireOptionalInt_NullValue(t *testing.T) {
	n, present, err := requireOptionalInt(map[string]any{"importance": nil}, "importance")
	require.NoError(t, err)
	require.False(t, present)
	require.Equal(t, 0, n)
}

func TestRequireOptionalInt_NativeNumber(t *testing.T) {
	n, present, err := requireOptionalInt(map[string]any{"importance": 1.0}, "importance")
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, 1, n)
}

func TestRequireOptionalInt_CoercibleString(t *testing.T) {
	// Defense-in-depth fallback still applies before erroring.
	n, present, err := requireOptionalInt(map[string]any{"limit": "5"}, "limit")
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, 5, n)
}

func TestRequireOptionalInt_UncoercibleValue_ReturnsLoudError(t *testing.T) {
	// This is the exact shape of the #1279/#1280 bug: a present-but-wrong-typed
	// value must produce an error, never a silent fallback to the default.
	_, present, err := requireOptionalInt(map[string]any{"importance": []any{"oops"}}, "importance")
	require.True(t, present, "the key was present — callers must not treat this as absent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "importance")
}

// ── requireOptionalBool: loud-error path for load-bearing flags ────────────

func TestRequireOptionalBool_AbsentKey(t *testing.T) {
	b, present, err := requireOptionalBool(map[string]any{}, "force")
	require.NoError(t, err)
	require.False(t, present)
	require.False(t, b)
}

func TestRequireOptionalBool_NullValue(t *testing.T) {
	b, present, err := requireOptionalBool(map[string]any{"force": nil}, "force")
	require.NoError(t, err)
	require.False(t, present)
	require.False(t, b)
}

func TestRequireOptionalBool_NativeBool(t *testing.T) {
	b, present, err := requireOptionalBool(map[string]any{"force": true}, "force")
	require.NoError(t, err)
	require.True(t, present)
	require.True(t, b)
}

func TestRequireOptionalBool_CoercibleString(t *testing.T) {
	b, present, err := requireOptionalBool(map[string]any{"force": "true"}, "force")
	require.NoError(t, err)
	require.True(t, present)
	require.True(t, b)
}

func TestRequireOptionalBool_UncoercibleValue_ReturnsLoudError(t *testing.T) {
	_, present, err := requireOptionalBool(map[string]any{"force": []any{"oops"}}, "force")
	require.True(t, present, "the key was present — callers must not treat this as absent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "force")
}

// ── toStringSlice(args, key): presence vs. wrong-type vs. valid array ──────

func TestToStringSlice_AbsentKey_ReturnsNilNil(t *testing.T) {
	tags, err := toStringSlice(map[string]any{}, "tags")
	require.NoError(t, err)
	require.Nil(t, tags)
}

func TestToStringSlice_NullValue_ReturnsNilNil(t *testing.T) {
	tags, err := toStringSlice(map[string]any{"tags": nil}, "tags")
	require.NoError(t, err)
	require.Nil(t, tags)
}

func TestToStringSlice_NativeArray_HappyPath(t *testing.T) {
	tags, err := toStringSlice(map[string]any{"tags": []any{"a", "b"}}, "tags")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, tags)
}

func TestToStringSlice_GoStringSlice_Accepted(t *testing.T) {
	// Go-native callers (test helpers, REST bridges, internal handler-to-handler
	// calls) construct args maps directly with []string rather than the []any
	// that JSON decoding produces. A []string IS an array of strings — it must
	// be accepted, not rejected as a wrong type. Regression guard for the
	// post-#1283 CI failure in TestHandleMemoryCorrect_PreservesSummaryOnTagOnlyChange,
	// whose helper passes tags as []string.
	tags, err := toStringSlice(map[string]any{"tags": []string{"go", "style"}}, "tags")
	require.NoError(t, err)
	require.Equal(t, []string{"go", "style"}, tags)
}

func TestToStringSlice_GoStringSlice_StillValidatesControlChars(t *testing.T) {
	// The []string path must go through the same control-character validation
	// as the []any path — acceptance of the type must not bypass #252 checks.
	_, err := toStringSlice(map[string]any{"tags": []string{"bad\x00tag"}}, "tags")
	require.Error(t, err)
	require.Contains(t, err.Error(), "disallowed control character")
}

func TestToStringSlice_GoStringSlice_Empty_ReturnsEmptyNotNil(t *testing.T) {
	// tags=[] has distinct semantics from tags omitted (memory_correct: []
	// clears valid_from, omitted preserves it). A Go-native empty []string
	// must behave like an empty []any: non-nil empty result, no error.
	tags, err := toStringSlice(map[string]any{"tags": []string{}}, "tags")
	require.NoError(t, err)
	require.NotNil(t, tags)
	require.Empty(t, tags)
}

func TestToStringSlice_JSONEncodedStringArray_Coerces(t *testing.T) {
	// Defense-in-depth: a client that still JSON-encodes the array as a string
	// despite the declared schema must not silently lose the tags (#1279).
	tags, err := toStringSlice(map[string]any{"tags": `["a","b"]`}, "tags")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, tags)
}

func TestToStringSlice_WrongType_ReturnsLoudErrorNotSilentNil(t *testing.T) {
	// The core #1279 regression: previously toStringSlice(v) returned (nil, nil)
	// for any non-[]any value, silently dropping the caller's tags. It must now
	// return an error naming the parameter.
	tags, err := toStringSlice(map[string]any{"tags": 12345.0}, "tags")
	require.Nil(t, tags)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tags")
}

func TestToStringSlice_GarbageString_ReturnsLoudError(t *testing.T) {
	tags, err := toStringSlice(map[string]any{"tags": "not-json"}, "tags")
	require.Nil(t, tags)
	require.Error(t, err)
}
