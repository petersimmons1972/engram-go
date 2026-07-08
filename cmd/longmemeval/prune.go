package main

// prune deletes LongMemEval scratch projects whose project_ttl.expires_at has
// elapsed. Intended to run as a periodic CronJob (deploy/lme-prune-cronjob.yaml)
// or ad-hoc from an operator shell.
//
// Schema deviation note (#754): projects are not a first-class table in
// engram-go — they are DISTINCT values of memories.project. TTL metadata is
// stored in the sidecar table project_ttl (migration 022). This CLI reads that
// sidecar table directly via PostgresBackend, then calls memory_delete_project
// over MCP for each expired entry so server-side invariants (cache
// invalidation, audit log) fire normally.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// DefaultScratchTTL is the default TTL applied to ephemeral lme-* projects at
// ingest time when no explicit --scratch-ttl is given. 7 days balances the
// cost of re-ingesting a haystack (~hours) against indefinite scratch
// accumulation that risks index bloat and accidental re-use.
const DefaultScratchTTL = 168 * time.Hour

// defaultScratchTTL returns the package-level default. Wrapped in a function
// so tests can pin the constant without referencing the literal.
func defaultScratchTTL() time.Duration {
	return DefaultScratchTTL
}

// PruneConfig holds the flags accepted by `longmemeval prune`.
type PruneConfig struct {
	// Prefix restricts deletion to projects whose names LIKE prefix+'%'. The
	// default "lme-" matches the ingest-stage naming convention.
	Prefix string

	// OlderThan shifts the effective cutoff to (now - OlderThan). A zero
	// duration means "use expires_at as-is" (anything strictly before now).
	OlderThan time.Duration

	// DryRun prints the projects that would be deleted but performs no
	// mutations.
	DryRun bool

	// Execute is the explicit opt-in that allows deletion. Without it, prune
	// always plans only, even when DryRun is false.
	Execute bool

	// ConfirmPrefix must match Prefix when Execute is true.
	ConfirmPrefix string

	// Limit caps the number of projects considered for deletion in a single
	// invocation. In execute mode this must be positive unless Unlimited is set.
	Limit int

	// Unlimited permits execute mode without a positive Limit.
	Unlimited bool

	// DatabaseURL is the PostgreSQL DSN; falls back to env DATABASE_URL.
	DatabaseURL string

	// ServerURL is the engram MCP endpoint used to call memory_delete_project.
	ServerURL string

	// APIKey is the engram bearer token.
	APIKey string

	// ExplicitAPIKey tracks whether credentials were supplied by a CLI flag
	// rather than automatic discovery.
	ExplicitAPIKey bool

	// UseDefaultToken permits automatic token discovery for execute mode.
	UseDefaultToken bool
}

// projectTTLEntry is the in-memory shape used by selectExpiredProjects. It
// mirrors the project_ttl table columns relevant to prune selection.
type projectTTLEntry struct {
	Name      string
	ExpiresAt *time.Time // nil = durable, never selected
}

// selectOption is a variadic option for selectExpiredProjects. It exists so
// that the older-than cutoff shift can stay an optional concern without
// bloating the function signature.
type selectOption func(*selectOptions)

// selectOptions is the resolved option struct consumed inside
// selectExpiredProjects.
type selectOptions struct {
	olderThan time.Duration
}

// withOlderThan overrides the effective cutoff to (now - d). A zero or
// negative duration is a no-op; callers should omit the option in that case.
func withOlderThan(d time.Duration) selectOption {
	return func(o *selectOptions) { o.olderThan = d }
}

// selectExpiredProjects filters entries down to those that are (a) prefixed
// with prefix, (b) have non-nil ExpiresAt, and (c) have ExpiresAt at or
// before the effective cutoff (now, or now-olderThan when withOlderThan is
// passed).
//
// When limit > 0, at most limit names are returned. The slice is in input
// order; callers that need a deterministic order should sort entries first.
func selectExpiredProjects(entries []projectTTLEntry, prefix string, limit int, now time.Time, opts ...selectOption) []string {
	var so selectOptions
	for _, opt := range opts {
		opt(&so)
	}
	cutoff := now
	if so.olderThan > 0 {
		cutoff = now.Add(-so.olderThan)
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ExpiresAt == nil {
			continue // durable — never prune
		}
		if !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		// Tests assert that exact-boundary (expires_at == cutoff) is included;
		// use !After rather than Before so cutoff itself is treated as expired.
		if e.ExpiresAt.After(cutoff) {
			continue
		}
		out = append(out, e.Name)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// runPruneWithEntries is the pure, side-effect-isolated core of the prune
// command. It receives the candidate entries, a delete function, and an output
// writer. Returns the process exit code.
//
// Returning 0 on an empty result set is intentional: the CronJob path runs on
// an interval and should not page when there is nothing to do.
func runPruneWithEntries(cfg *PruneConfig, entries []projectTTLEntry, deleteFn func(string) error, now time.Time, out io.Writer) int {
	var opts []selectOption
	if cfg.OlderThan > 0 {
		opts = append(opts, withOlderThan(cfg.OlderThan))
	}
	victims := selectExpiredProjects(entries, cfg.Prefix, cfg.Limit, now, opts...)

	if len(victims) == 0 {
		_, _ = fmt.Fprintln(out, "prune: no expired projects matching prefix")
		return 0
	}

	if effectiveDryRun(cfg) {
		_, _ = fmt.Fprintf(out, "prune: DRY RUN — would delete %d project(s):\n", len(victims))
		for _, name := range victims {
			_, _ = fmt.Fprintf(out, "  %s\n", name)
		}
		return 0
	}

	var firstErr error
	deleted := 0
	for _, name := range victims {
		if err := deleteFn(name); err != nil {
			_, _ = fmt.Fprintf(out, "prune: delete %q failed: %v\n", name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deleted++
		_, _ = fmt.Fprintf(out, "prune: deleted %s\n", name)
	}
	_, _ = fmt.Fprintf(out, "prune: %d of %d project(s) deleted\n", deleted, len(victims))
	if firstErr != nil {
		return 1
	}
	return 0
}

func effectiveDryRun(cfg *PruneConfig) bool {
	return cfg.DryRun || !cfg.Execute
}

func validatePruneConfig(cfg *PruneConfig) error {
	if cfg.Prefix == "" {
		return errors.New("prune: --prefix is required")
	}
	if cfg.Limit < 0 {
		return errors.New("prune: --limit must be >= 0")
	}
	if cfg.Execute && cfg.DryRun {
		return errors.New("prune: --execute and --dry-run cannot be combined")
	}
	if !cfg.Execute {
		return nil
	}
	if cfg.ConfirmPrefix != cfg.Prefix {
		return fmt.Errorf("prune: --confirm-prefix must exactly match --prefix (%q)", cfg.Prefix)
	}
	if cfg.Unlimited && cfg.Limit > 0 {
		return errors.New("prune: use either --limit N or --unlimited, not both")
	}
	if !cfg.Unlimited && cfg.Limit <= 0 {
		return errors.New("prune: --execute requires --limit N > 0 or --unlimited")
	}
	if !cfg.ExplicitAPIKey && !cfg.UseDefaultToken {
		return errors.New("prune: --execute requires --api-key, or --use-default-token to allow discovered credentials")
	}
	if cfg.APIKey == "" {
		return errors.New("prune: --execute requires a non-empty API key")
	}
	return nil
}

// runPrune is the wired-up entrypoint that connects to Postgres, lists
// expired projects from project_ttl, and calls memory_delete_project over MCP
// for each. Errors during list/connect return non-zero exit; per-delete errors
// are aggregated by runPruneWithEntries.
func runPrune(cfg *PruneConfig, stdout, stderr io.Writer) int {
	if err := validatePruneConfig(cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if cfg.DatabaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "prune: DATABASE_URL or --database-url is required")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	backend, err := db.NewPostgresBackend(ctx, "_prune", cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "prune: connect: %v\n", err)
		return 1
	}
	defer backend.Close()

	now := time.Now()
	cutoff := now
	if cfg.OlderThan > 0 {
		cutoff = now.Add(-cfg.OlderThan)
	}
	names, err := backend.ListExpiredProjects(ctx, cfg.Prefix, cutoff, cfg.Limit)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "prune: list expired: %v\n", err)
		return 1
	}

	// ListExpiredProjects already filters by expires_at < cutoff and prefix.
	// Synthesise a past-expires marker so the in-memory selectExpiredProjects
	// re-filter is a no-op (defence in depth against SQL drift).
	pastMarker := now.Add(-time.Second)
	entries := make([]projectTTLEntry, 0, len(names))
	for _, n := range names {
		entries = append(entries, projectTTLEntry{Name: n, ExpiresAt: &pastMarker})
	}

	deleteFn := func(name string) error {
		return errors.New("prune: delete client not initialised — use --dry-run")
	}
	if !effectiveDryRun(cfg) {
		client, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "prune: MCP connect: %v\n", err)
			return 1
		}
		defer func() { _ = client.Close() }()
		deleteFn = func(name string) error {
			return client.DeleteProject(ctx, name)
		}
	}

	return runPruneWithEntries(cfg, entries, deleteFn, now, stdout)
}

// dispatchPrune handles the `prune` subcommand: parses flags and invokes
// runPrune. Returns the process exit code.
func dispatchPrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &PruneConfig{}
	fs.StringVar(&cfg.Prefix, "prefix", "lme-", "delete only projects whose names start with this prefix")
	fs.DurationVar(&cfg.OlderThan, "older-than", 0, "additional cushion past expires_at before pruning (e.g. 24h); 0 means use expires_at as-is")
	fs.DurationVar(&cfg.OlderThan, "scratch-ttl", 0, "alias for --older-than; documents intent for lme scratch retention windows")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "print projects that would be deleted but make no changes (default behavior)")
	fs.BoolVar(&cfg.Execute, "execute", false, "delete matching projects; requires --confirm-prefix and --limit N or --unlimited")
	fs.StringVar(&cfg.ConfirmPrefix, "confirm-prefix", "", "required with --execute; must exactly match --prefix")
	fs.IntVar(&cfg.Limit, "limit", 0, "cap deletions to this many projects per invocation; required with --execute unless --unlimited")
	fs.BoolVar(&cfg.Unlimited, "unlimited", false, "allow --execute without a deletion limit")
	fs.StringVar(&cfg.DatabaseURL, "database-url", envOr("DATABASE_URL", ""), "PostgreSQL DSN (env: DATABASE_URL)")
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	fs.BoolVar(&cfg.UseDefaultToken, "use-default-token", false, "allow --execute to use ENGRAM_API_KEY or Claude MCP config token")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: longmemeval prune [flags]")
		_, _ = fmt.Fprintln(stderr)
		_, _ = fmt.Fprintln(stderr, "Delete LongMemEval scratch projects whose project_ttl.expires_at has elapsed.")
		_, _ = fmt.Fprintln(stderr)
		_, _ = fmt.Fprintln(stderr, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	cfg.ExplicitAPIKey = longmemeval.FlagWasProvided(fs, "api-key")
	if !cfg.ExplicitAPIKey && cfg.UseDefaultToken {
		cfg.APIKey = longmemeval.DefaultAPIKey()
	}
	if !longmemeval.FlagWasProvided(fs, "url") {
		cfg.ServerURL = longmemeval.DefaultServerURL()
	}

	return runPrune(cfg, stdout, stderr)
}
