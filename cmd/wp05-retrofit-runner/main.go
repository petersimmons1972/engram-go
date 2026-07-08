// Command wp05-retrofit-runner drives the internal/wp05retrofit harness
// end-to-end against a live Engram server: load a LongMemEval-shaped
// fixture, ingest+recall+score each item (see wp05retrofit.Run), and write
// the resulting Bundle as JSON.
//
// This is WP-0.5c, the "retrofit" arm of the WP-0.5 retrofit-vs-greenfield
// bake-off (see the internal/wp05retrofit package doc for full campaign
// context and the duramind JSON-compatibility contract the output must
// hold). It is bake-off scaffolding: expected to be retired once WP-0.5
// concludes, not permanent CLI surface.
//
// Usage:
//
//	wp05-retrofit-runner -data <fixture.json> -url <engram-url> -api-key <key> \
//	    -project-prefix wp05-retrofit -limit 200 -out results/wp05-retrofit/retrofit-bundle.json
//
// -url and -api-key default to the local Engram MCP server config
// (~/.claude/mcp_servers.json) when not provided, mirroring cmd/longmemeval's
// discovery behavior (see longmemeval.MCPDefaults). Provenance.HarnessSHA is resolved in
// priority order: -harness-sha override, `git rev-parse --short HEAD`, the
// binary's embedded VCS build-info revision, else "unknown" — see
// resolveHarnessSHA; per issue #1320, a missing SHA is informational and
// must never abort the run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/wp05retrofit"
)

func main() {
	var (
		dataPath              = flag.String("data", "/tmp/lme_s_multisession_133.json", "path to the LME-S multi-session fixture JSON")
		serverURL             = flag.String("url", "", "Engram server URL (env: ENGRAM_URL)")
		apiKey                = flag.String("api-key", "", "Engram API key (env: ENGRAM_API_KEY)")
		projectPrefix         = flag.String("project-prefix", "wp05-retrofit", "project namespace prefix; each fixture item gets its own <prefix>-<question_id> project")
		limit                 = flag.Int("limit", 200, "recall limit; must exceed the max haystack session count per item")
		exhaustiveAggregation = flag.Bool("exhaustive-aggregation", false, "H8: for aggregation questions, union topK=500 primary+anchor recall with a project-wide memory_list sweep and build Layer B client-side")
		skipIngest            = flag.Bool("skip-ingest", false, "recall/score only — assumes memories already ingested under project-prefix")
		outPath               = flag.String("out", "results/wp05-retrofit/retrofit-bundle.json", "output path for the retrofit bundle JSON")
		harnessSHAFlag        = flag.String("harness-sha", "", "override the harness SHA recorded in provenance; skips git rev-parse and the build-info fallback (useful outside a git checkout, e.g. a shallow clone or extracted tarball)")
	)
	flag.Parse()
	applySharedDefaults(flag.CommandLine, serverURL, apiKey)

	if err := run(*dataPath, *serverURL, *apiKey, *projectPrefix, *limit, *exhaustiveAggregation, *skipIngest, *outPath, *harnessSHAFlag); err != nil {
		fmt.Fprintf(os.Stderr, "wp05-retrofit-runner: %v\n", err)
		os.Exit(1)
	}
}

func run(dataPath, serverURL, apiKey, projectPrefix string, limit int, exhaustiveAggregation, skipIngest bool, outPath, harnessSHAOverride string) error {
	items, err := wp05retrofit.LoadFixture(dataPath)
	if err != nil {
		return fmt.Errorf("load fixture: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	client, err := longmemeval.Connect(ctx, serverURL, apiKey)
	if err != nil {
		return fmt.Errorf("connect MCP: %w", err)
	}
	defer func() { _ = client.Close() }()

	harnessSHA := resolveHarnessSHA(ctx, harnessSHAOverride, gitShortSHA, debug.ReadBuildInfo)
	runID := "wp05-retrofit-" + time.Now().UTC().Format("20060102T150405Z")
	itemSet := fmt.Sprintf("%s-develop-n%d", strings.TrimSuffix(filepath.Base(dataPath), filepath.Ext(dataPath)), len(items))
	featureFlags := []string{"layer_b_retrofit"}
	if exhaustiveAggregation {
		featureFlags = append(featureFlags, "exhaustive_aggregation")
	}
	provenance := wp05retrofit.Provenance{
		GoldVersion:   filepath.Base(dataPath),
		ScorerVersion: "wp05-retrofit-v1",
		FeatureFlags:  featureFlags,
		System:        wp05retrofit.SystemName,
		ItemSet:       itemSet,
		RunID:         runID,
		HarnessSHA:    harnessSHA,
	}

	log.Printf("wp05-retrofit-runner: starting %d fixture item(s), project-prefix=%s limit=%d exhaustive=%t skip-ingest=%t url=%s",
		len(items), projectPrefix, limit, exhaustiveAggregation, skipIngest, serverURL)

	bundle, err := wp05retrofit.Run(ctx, client, items, wp05retrofit.Config{
		ProjectPrefix:         projectPrefix,
		Limit:                 limit,
		ExhaustiveAggregation: exhaustiveAggregation,
		SkipIngest:            skipIngest,
		ProvenanceTemplate:    provenance,
	}, log.Printf)
	if err != nil {
		return err
	}

	if err := wp05retrofit.WriteBundle(outPath, bundle); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	log.Printf("wp05-retrofit-runner: wrote retrofit bundle to %s (%d items)", outPath, len(bundle.Items))
	return nil
}

func applySharedDefaults(fs *flag.FlagSet, serverURL, apiKey *string) {
	if !longmemeval.FlagWasProvided(fs, "url") {
		*serverURL = longmemeval.DefaultServerURL()
	}
	if !longmemeval.FlagWasProvided(fs, "api-key") {
		*apiKey = longmemeval.DefaultAPIKey()
	}
}

// harnessSHAUnknown is recorded when no SHA can be resolved by any means
// (no git checkout, no VCS build-info stamp, and no --harness-sha override).
// The eval tool must degrade gracefully rather than abort the whole run for
// a provenance field that is informational, not load-bearing (#1320).
const harnessSHAUnknown = "unknown"

// resolveHarnessSHA determines the harness SHA to record in provenance,
// trying each source in order and falling back to the next on failure:
//  1. an explicit --harness-sha override, if provided
//  2. `git rev-parse --short HEAD` (runGit), when run inside a git checkout
//  3. the VCS revision embedded in the binary's build info (readBuildInfo),
//     available when the binary was built via `go build`/`go install` from
//     within a git checkout even if one isn't present at run time
//  4. harnessSHAUnknown, if none of the above resolve
//
// runGit and readBuildInfo are parameters (rather than direct calls to
// gitShortSHA/debug.ReadBuildInfo) so tests can exercise the fallback chain
// without depending on the actual git/build-info state of the test runner.
func resolveHarnessSHA(ctx context.Context, override string, runGit func(context.Context) (string, error), readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if override != "" {
		return override
	}
	if sha, err := runGit(ctx); err == nil {
		if sha = strings.TrimSpace(sha); sha != "" {
			return sha
		}
	}
	if readBuildInfo != nil {
		if info, ok := readBuildInfo(); ok && info != nil {
			for _, setting := range info.Settings {
				if setting.Key != "vcs.revision" {
					continue
				}
				rev := strings.TrimSpace(setting.Value)
				if rev == "" {
					continue
				}
				if len(rev) > 12 {
					rev = rev[:12]
				}
				return rev
			}
		}
	}
	return harnessSHAUnknown
}

func gitShortSHA(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
