package longmemeval

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSanitizeOAIDebugBodyRedactsSensitiveContent(t *testing.T) {
	body := []byte(`{"error":"bad request","prompt":"private memory context","api_key":"sk-test-secret","messages":[{"content":"user profile details"}]}`)

	got := sanitizeOAIDebugBody(body)

	for _, sensitive := range []string{
		"private memory context",
		"sk-test-secret",
		"user profile details",
		`"prompt"`,
		`"messages"`,
	} {
		if strings.Contains(got, sensitive) {
			t.Fatalf("sanitized debug body leaked %q in %q", sensitive, got)
		}
	}
	if !strings.Contains(got, "bytes=") {
		t.Fatalf("sanitized debug body should retain safe size metadata, got %q", got)
	}
}

func TestRunCodex_ForwardsExactArgumentsAndReturnsFinalAssistantBlock(t *testing.T) {
	binDir := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "args")
	argcPath := filepath.Join(t.TempDir(), "argc")
	writeFakeCodex(t, binDir, `
printf '%s\n' "$@" > "$CODEX_ARGS_FILE"
printf '%s' "$#" > "$CODEX_ARGC_FILE"
printf '\033[31massistant/analysis\033[0m\nignore this reasoning\n'
printf '\033[32massistant/final\033[0m\n  frontier answer  \n'
`)
	t.Setenv("PATH", binDir)
	t.Setenv("CODEX_ARGS_FILE", argsPath)
	t.Setenv("CODEX_ARGC_FILE", argcPath)

	const prompt = "question with\nidentical context"
	got, err := runCodex(context.Background(), prompt, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	if got != "frontier answer" {
		t.Fatalf("runCodex() = %q, want %q", got, "frontier answer")
	}

	rawArgs, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	gotArgs := strings.Split(strings.TrimSuffix(string(rawArgs), "\n"), "\n")
	wantArgs := []string{
		"exec",
		"--model",
		"gpt-5.6-sol",
		"-c",
		"model_reasoning_effort=high",
		"--sandbox",
		"read-only",
		"question with",
		"identical context",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("codex args = %#v, want %#v", gotArgs, wantArgs)
	}
	argc, err := os.ReadFile(argcPath)
	if err != nil {
		t.Fatalf("read captured argc: %v", err)
	}
	if string(argc) != "8" {
		t.Fatalf("codex argc = %q, want 8 (prompt must remain one argument)", argc)
	}
}

func TestRunCodex_CodexBinaryMissingReturnsNonNilError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err == nil {
		t.Fatalf("runCodex() error = nil, want missing-binary error; output=%q", got)
	}
	if got != "" {
		t.Fatalf("runCodex() output = %q, want empty on error", got)
	}
}

func TestRunCodex_ExecErrorReturnsNonNilError(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `
printf 'synthetic codex failure' >&2
exit 17
`)
	t.Setenv("PATH", binDir)

	_, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err == nil {
		t.Fatal("runCodex() error = nil, want nonzero-exit error")
	}
	if !strings.Contains(err.Error(), "synthetic codex failure") {
		t.Fatalf("runCodex() error = %q, want stderr context", err)
	}
}

func TestRunCodex_EmptyOrANSIOnlyStdoutReturnsError(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "empty stdout", output: ""},
		{name: "ANSI color-only stdout", output: `printf '\033[31m\033[0m\n'`},
		{name: "ANSI title-only stdout", output: `printf '\033]0;private title\007'`},
		{name: "8-bit ANSI title-only stdout", output: `printf '\2350;private title\234'`},
		{name: "ANSI DCS-only stdout", output: `printf '\033Pprivate payload\033\\'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			writeFakeCodex(t, binDir, tt.output)
			t.Setenv("PATH", binDir)

			got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
			if err == nil {
				t.Fatalf("runCodex() error = nil, want empty-output error; output=%q", got)
			}
		})
	}
}

func TestRunCodex_ANSIControlStringsDoNotConsumeVisibleText(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf '\033]0;first\033\\visible\033]0;second\007'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	if got != "visible" {
		t.Fatalf("runCodex() = %q, want visible text between ANSI controls", got)
	}
}

func TestRunCodex_PlainFinalTextPreservesTokensUsedLine(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf 'The phrase\ntokens used\nbelongs in this answer.\n'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	want := "The phrase\ntokens used\nbelongs in this answer."
	if got != want {
		t.Fatalf("runCodex() = %q, want %q", got, want)
	}
}

func TestRunCodex_PlainTextPreservesStandaloneCodexLines(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf 'codex\nUse this heading:\ncodex\ncontinued\n'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	want := "codex\nUse this heading:\ncodex\ncontinued"
	if got != want {
		t.Fatalf("runCodex() = %q, want %q", got, want)
	}
}

func TestRunCodex_FramedAnswerPreservesCodexLineInsideBlock(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf 'codex\nUse this heading:\ncodex\ncontinued\ntokens used\n42\n'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	want := "Use this heading:\ncodex\ncontinued"
	if got != want {
		t.Fatalf("runCodex() = %q, want %q", got, want)
	}
}

func TestRunCodex_FramedAnswerPreservesTokensUsedLineInsideBlock(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf 'codex\nThe phrase\ntokens used\nbelongs here.\ntokens used\n42\n'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	want := "The phrase\ntokens used\nbelongs here."
	if got != want {
		t.Fatalf("runCodex() = %q, want %q", got, want)
	}
}

func TestRunCodex_NumericTextAfterTokensUsedIsNotFooter(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `printf 'codex\nThe phrase\ntokens used\n42\nwhich was low\n'`)
	t.Setenv("PATH", binDir)

	got, err := runCodex(context.Background(), "prompt", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	want := "codex\nThe phrase\ntokens used\n42\nwhich was low"
	if got != want {
		t.Fatalf("runCodex() = %q, want %q", got, want)
	}
}

func TestRunCodex_ContextTimeoutReturnsError(t *testing.T) {
	if codexExecTimeout != 300*time.Second {
		t.Fatalf("codexExecTimeout = %s, want 300s/item", codexExecTimeout)
	}
	binDir := t.TempDir()
	writeFakeCodex(t, binDir, `exec /bin/sleep 10`)
	t.Setenv("PATH", binDir)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runCodex(ctx, "prompt", "gpt-5.6-sol")
	if err == nil {
		t.Fatal("runCodex() error = nil, want context timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("runCodex() ignored context cancellation for %s", elapsed)
	}
}

func writeFakeCodex(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, "codex")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
}
