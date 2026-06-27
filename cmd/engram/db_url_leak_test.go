package main

// Tests for #1212: DB password leaks via --database-url flag.
//
// Three attack surfaces eliminated:
//  1. --database-url flag registers the DSN (with embedded password) as a
//     positional CLI argument visible in /proc/cmdline, ps aux, and --help.
//  2. --help PrintDefaults expands the DATABASE_URL env var into the default
//     value string, printing the DSN in plaintext.
//  3. runHealthcheckProbe calls os.Exit before the os.Unsetenv block, leaving
//     ENGRAM_API_KEY / DATABASE_URL / LITELLM_API_KEY in /proc/self/environ.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDatabaseURLFlagAbsent verifies that --database-url is not a registered
// CLI flag in runServer.  Pre-fix: fs.String("database-url", ...) registered
// the flag so a DSN like "postgres://user:password@host/db" would appear in
// /proc/cmdline and in --help output.  Post-fix: DATABASE_URL is env-only.
// (#1212)
func TestDatabaseURLFlagAbsent(t *testing.T) {
	// Spin up the binary (via the TestEngramMainHelper subprocess gate) and pass
	// --database-url; the flag package should reject it with "flag provided but
	// not defined" if the flag has been removed correctly.
	out, err := runEngramMainForTest(t, "server", "--database-url", "postgres://user:secret@host/db")
	if err == nil {
		t.Fatalf("runServer --database-url unexpectedly succeeded; output:\n%s", out)
	}
	if !strings.Contains(out, "flag provided but not defined") {
		t.Fatalf("expected 'flag provided but not defined' for --database-url, got:\n%s", out)
	}
}

// TestDatabaseURLHelpDoesNotExpandSecret verifies that running --help on the
// server subcommand does not expand the DATABASE_URL environment variable into
// flag default-value text.  Pre-fix: fs.String("database-url", envOr(...), ...)
// bakes the env value into the flag default, which PrintDefaults then prints.
// Post-fix: DATABASE_URL is read directly (not via flag.String), so it never
// appears in --help output.  (#1212)
func TestDatabaseURLHelpDoesNotExpandSecret(t *testing.T) {
	// Inject a recognisable sentinel that would appear in --help if the flag
	// default expands DATABASE_URL.
	const sentinelDSN = "postgres://leaktest:SUPER_SECRET_PWD@db.internal/engram"

	// Build the subprocess command with a specific DATABASE_URL value.
	// We can't use runEngramMainForTest directly because it clears DATABASE_URL.
	cmd := exec.Command(os.Args[0], "-test.run=TestEngramMainHelper", "--", "server", "--help")
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MAIN_HELPER=1",
		"DATABASE_URL="+sentinelDSN,
	)
	out, _ := cmd.CombinedOutput()

	if strings.Contains(string(out), "SUPER_SECRET_PWD") {
		t.Fatalf("--help output contains embedded DB password from DATABASE_URL; output:\n%s", out)
	}
	if strings.Contains(string(out), sentinelDSN) {
		t.Fatalf("--help output contains full DATABASE_URL DSN; output:\n%s", out)
	}
}

// TestUnsetenvBeforeHealthcheck verifies (via source inspection) that the
// os.Unsetenv block for secret env vars appears before the runHealthcheckProbe
// call in main.go.  Pre-fix: runHealthcheckProbe called os.Exit, so the Unsetenv
// block after it was unreachable and secrets remained in /proc/self/environ.
// Post-fix: Unsetenv block is moved above the healthcheck branch.  (#1212)
//
// Source inspection is the right tool here: the behaviour under test is
// call-ordering across an os.Exit boundary that cannot be exercised in-process.
func TestUnsetenvBeforeHealthcheck(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)

	unsetIdx := strings.Index(text, `os.Unsetenv("DATABASE_URL")`)
	hcIdx := strings.Index(text, "runHealthcheckProbe(")
	if unsetIdx < 0 {
		t.Fatal("main.go: os.Unsetenv(\"DATABASE_URL\") not found — has the unset block been removed?")
	}
	if hcIdx < 0 {
		t.Fatal("main.go: runHealthcheckProbe call not found")
	}
	if unsetIdx > hcIdx {
		t.Errorf("os.Unsetenv(\"DATABASE_URL\") (offset %d) appears AFTER runHealthcheckProbe (offset %d) in main.go — secrets remain in /proc/self/environ when healthcheck exits (#1212)", unsetIdx, hcIdx)
	}
}
