package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDispatch_Help — #662: `longmemeval help` must exit 0 and print usage.
// Currently the binary prints `unknown subcommand "help"` to stderr and exits 1.
func TestDispatch_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "help"}, &stdout, &stderr)

	if exit != 0 {
		t.Errorf("`longmemeval help` exit = %d, want 0", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("help output missing 'Usage:' header\n--- stdout ---\n%s\n--- stderr ---\n%s",
			out, stderr.String())
	}
	for _, sub := range []string{"ingest", "run", "score", "all"} {
		if !strings.Contains(out, sub) {
			t.Errorf("help output missing subcommand %q in listing", sub)
		}
	}
	if strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("help should NOT report itself as an unknown subcommand: %s", stderr.String())
	}
}

// TestDispatch_NoArgs — invoking with no subcommand should print usage to stderr
// and exit non-zero (matches existing behaviour for unknown subcommands).
func TestDispatch_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval"}, &stdout, &stderr)

	if exit == 0 {
		t.Errorf("`longmemeval` with no subcommand must exit non-zero, got 0")
	}
	if !strings.Contains(stderr.String()+stdout.String(), "Usage") {
		t.Errorf("no-args invocation must print usage somewhere; stderr=%q stdout=%q",
			stderr.String(), stdout.String())
	}
}

// TestDispatch_UnknownSubcommand — preserves existing error-on-unknown behaviour.
func TestDispatch_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "frobnicate"}, &stdout, &stderr)

	if exit == 0 {
		t.Error("unknown subcommand must exit non-zero")
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand' in stderr, got %q", stderr.String())
	}
}

func TestHelp_RunSubcommandDoesNotLeakResolvedAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ENGRAM_API_KEY", "engram-secret-for-help-test")

	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "run", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("dispatch(run --help) exit = %d, want 0", exit)
	}

	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "-api-key") {
		t.Fatalf("help output missing -api-key flag: %q", combined)
	}
	if strings.Contains(combined, "engram-secret-for-help-test") {
		t.Fatalf("help output leaked resolved API key:\n%s", combined)
	}
}

func TestHelp_RunSubcommandDocumentsFullTimelineContext(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "run", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("dispatch(run --help) exit = %d, want 0", exit)
	}

	combined := stdout.String() + stderr.String()
	for _, want := range []string{"-full-timeline-context", "benchmark-only", "memory_recall", "memory_fetch"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("run help missing %q:\n%s", want, combined)
		}
	}
}

func TestHelp_RunSubcommandDocumentsGeneratorFlagsAndDefaults(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "run", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("dispatch(run --help) exit = %d, want 0", exit)
	}

	combined := stdout.String() + stderr.String()
	for _, want := range []string{"-generator", `default "vllm"`, "-generator-model", `default "gpt-5.6-sol"`} {
		if !strings.Contains(combined, want) {
			t.Fatalf("run help missing %q:\n%s", want, combined)
		}
	}
}

func TestDispatch_RunUnknownGeneratorFailsAtStartup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch(
		[]string{"longmemeval", "run", "--generator", "unknown"},
		&stdout,
		&stderr,
	)

	if exit == 0 {
		t.Fatalf("dispatch(run --generator unknown) exit = 0, want nonzero")
	}
	if !strings.Contains(stderr.String(), `--generator "unknown"`) || !strings.Contains(stderr.String(), "vllm, codex") {
		t.Fatalf("unknown-generator error is unclear: %q", stderr.String())
	}
}

func TestDispatch_SpecialSubcommandsReturnControlledHelpAndParseExitCodes(t *testing.T) {
	if os.Getenv("LONGMEMEVAL_DISPATCH_HELPER") == "1" {
		args := strings.Split(os.Getenv("LONGMEMEVAL_DISPATCH_ARGS"), "\n")
		os.Exit(dispatch(args, os.Stdout, os.Stderr))
	}

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantText string
	}{
		{name: "score-efficient help", args: []string{"longmemeval", "score-efficient", "--help"}, wantExit: 0, wantText: "Usage of score-efficient:"},
		{name: "score-batch help", args: []string{"longmemeval", "score-batch", "--help"}, wantExit: 0, wantText: "Usage of score-batch:"},
		{name: "sample-prepare help", args: []string{"longmemeval", "sample-prepare", "--help"}, wantExit: 0, wantText: "Usage of sample-prepare:"},
		{name: "sample-analyze help", args: []string{"longmemeval", "sample-analyze", "--help"}, wantExit: 0, wantText: "Usage of sample-analyze:"},
		{name: "route-discover help", args: []string{"longmemeval", "route-discover", "--help"}, wantExit: 0, wantText: "Usage of route-discover:"},
		{name: "score-efficient parse error", args: []string{"longmemeval", "score-efficient", "--badflag"}, wantExit: 2, wantText: "flag provided but not defined"},
		{name: "score-batch parse error", args: []string{"longmemeval", "score-batch", "--badflag"}, wantExit: 2, wantText: "flag provided but not defined"},
		{name: "sample-prepare parse error", args: []string{"longmemeval", "sample-prepare", "--badflag"}, wantExit: 2, wantText: "flag provided but not defined"},
		{name: "sample-analyze parse error", args: []string{"longmemeval", "sample-analyze", "--badflag"}, wantExit: 2, wantText: "flag provided but not defined"},
		{name: "route-discover parse error", args: []string{"longmemeval", "route-discover", "--badflag"}, wantExit: 2, wantText: "flag provided but not defined"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestDispatch_SpecialSubcommandsReturnControlledHelpAndParseExitCodes")
			cmd.Env = append(os.Environ(),
				"LONGMEMEVAL_DISPATCH_HELPER=1",
				"LONGMEMEVAL_DISPATCH_ARGS="+strings.Join(tc.args, "\n"),
			)
			out, err := cmd.CombinedOutput()

			if tc.wantExit == 0 {
				if err != nil {
					t.Fatalf("subprocess err = %v, output=%s", err, out)
				}
			} else {
				var exitErr *exec.ExitError
				if !asExitError(err, &exitErr) {
					t.Fatalf("want exit %d, err=%v, output=%s", tc.wantExit, err, out)
				}
				if exitErr.ExitCode() != tc.wantExit {
					t.Fatalf("exit = %d, want %d, output=%s", exitErr.ExitCode(), tc.wantExit, out)
				}
			}

			if !strings.Contains(string(out), tc.wantText) {
				t.Fatalf("output missing %q:\n%s", tc.wantText, out)
			}
		})
	}
}

func TestDispatch_APIKeyDefaultsDoNotLeakInHelp(t *testing.T) {
	for _, subcommand := range []string{"ingest", "run"} {
		t.Run(subcommand+"/env", func(t *testing.T) {
			const secret = "env-secret-must-not-print"
			t.Setenv("ENGRAM_API_KEY", secret)
			t.Setenv("HOME", t.TempDir())

			var stdout, stderr bytes.Buffer
			exit := dispatch([]string{"longmemeval", subcommand, "--help"}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("%s --help exit = %d, want 0; stderr=%q", subcommand, exit, stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, secret) {
				t.Fatalf("%s --help leaked ENGRAM_API_KEY in output:\n%s", subcommand, combined)
			}
		})

		t.Run(subcommand+"/mcp", func(t *testing.T) {
			const secret = "mcp-secret-must-not-print"
			home := t.TempDir()
			t.Setenv("ENGRAM_API_KEY", "")
			t.Setenv("HOME", home)
			writeClaudeMCPConfig(t, home, secret)

			var stdout, stderr bytes.Buffer
			exit := dispatch([]string{"longmemeval", subcommand, "--help"}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("%s --help exit = %d, want 0; stderr=%q", subcommand, exit, stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, secret) {
				t.Fatalf("%s --help leaked Claude MCP bearer token in output:\n%s", subcommand, combined)
			}
		})
	}
}

func TestDispatch_ScoreEfficientHelpDoesNotLeakScorerAPIKeyAndWarnsAboutArgv(t *testing.T) {
	const secret = "scorer-secret-must-not-print"
	t.Setenv("LME_SCORER_API_KEY", secret)

	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "score-efficient", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("score-efficient --help exit = %d, want 0; stderr=%q", exit, stderr.String())
	}

	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, secret) {
		t.Fatalf("score-efficient --help leaked LME_SCORER_API_KEY in output:\n%s", combined)
	}
	if !strings.Contains(combined, "avoid secrets on argv") {
		t.Fatalf("score-efficient --help missing argv warning:\n%s", combined)
	}
}

func TestApplySharedDefaultsPreservesExplicitEmptyValues(t *testing.T) {
	t.Setenv("ENGRAM_API_KEY", "env-secret")
	t.Setenv("ENGRAM_URL", "http://env.example.test")
	t.Setenv("HOME", t.TempDir())

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	if err := fs.Parse([]string{"--api-key=", "--url="}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applySharedDefaults(cfg, fs)

	if cfg.APIKey != "" {
		t.Fatalf("explicit empty --api-key must remain empty, got %q", cfg.APIKey)
	}
	if cfg.ServerURL != "" {
		t.Fatalf("explicit empty --url must remain empty, got %q", cfg.ServerURL)
	}
}

func TestApplySharedDefaultsUsesEnvironmentWhenFlagsOmitted(t *testing.T) {
	t.Setenv("ENGRAM_API_KEY", "env-secret")
	t.Setenv("ENGRAM_URL", "http://env.example.test")
	t.Setenv("HOME", t.TempDir())

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applySharedDefaults(cfg, fs)

	if cfg.APIKey != "env-secret" {
		t.Fatalf("omitted --api-key should use ENGRAM_API_KEY, got %q", cfg.APIKey)
	}
	if cfg.ServerURL != "http://env.example.test" {
		t.Fatalf("omitted --url should use ENGRAM_URL, got %q", cfg.ServerURL)
	}
}

func TestApplyScoreEfficientDefaultsUsesEnvironmentWhenFlagOmitted(t *testing.T) {
	t.Setenv("LME_SCORER_API_KEY", "lme-secret")
	t.Setenv("OPENAI_API_KEY", "openai-secret")

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.ScorerAPIKey, "scorer-api-key", "", "scorer API key")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applyScoreEfficientDefaults(cfg, fs)

	if cfg.ScorerAPIKey != "lme-secret" {
		t.Fatalf("omitted --scorer-api-key should use LME_SCORER_API_KEY first, got %q", cfg.ScorerAPIKey)
	}
}

func TestApplyScoreEfficientDefaultsPreservesExplicitFlagValue(t *testing.T) {
	t.Setenv("LME_SCORER_API_KEY", "lme-secret")

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.ScorerAPIKey, "scorer-api-key", "", "scorer API key")
	if err := fs.Parse([]string{"--scorer-api-key=argv-secret"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applyScoreEfficientDefaults(cfg, fs)

	if cfg.ScorerAPIKey != "argv-secret" {
		t.Fatalf("explicit --scorer-api-key must remain unchanged, got %q", cfg.ScorerAPIKey)
	}
}

func TestDispatchRejectsInvalidScoreOutputMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "score", "--data", "questions.json", "--score-output", "xml"}, &stdout, &stderr)

	if exit == 0 {
		t.Fatal("dispatch accepted invalid --score-output")
	}
	if !strings.Contains(stderr.String(), "--score-output") {
		t.Fatalf("stderr = %q, want --score-output validation error", stderr.String())
	}
}

func TestConfig_OverlapGuardRejectsHalfOrMore(t *testing.T) {
	for _, value := range []string{"4000", "4001"} {
		t.Run(value, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exit := dispatch([]string{
				"longmemeval",
				"ingest",
				"--data", "questions.json",
				"--block-overlap-chars", value,
			}, &stdout, &stderr)

			if exit == 0 {
				t.Fatalf("dispatch accepted --block-overlap-chars=%s", value)
			}
			if !strings.Contains(stderr.String(), "--block-overlap-chars must be < 4000") {
				t.Fatalf("stderr = %q, want overlap guard message", stderr.String())
			}
		})
	}
}

func writeClaudeMCPConfig(t *testing.T, home, token string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	data := `{"mcpServers":{"engram":{"url":"http://example.test:8788/sse","headers":{"Authorization":"Bearer ` + token + `"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "mcp_servers.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}
}

// silenceWriter discards writes — used as a placeholder for tests that don't
// inspect output but need to satisfy a writer parameter.
type silenceWriter struct{}

func (silenceWriter) Write(b []byte) (int, error) { return len(b), nil }

var _ io.Writer = silenceWriter{}
