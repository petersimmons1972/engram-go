package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/wp05retrofit"
)

func main() {
	var (
		dataPath      = flag.String("data", "/tmp/lme_s_multisession_133.json", "path to the LME-S multi-session fixture JSON")
		serverURL     = flag.String("url", "", "Engram server URL (env: ENGRAM_URL)")
		apiKey        = flag.String("api-key", "", "Engram API key (env: ENGRAM_API_KEY)")
		projectPrefix = flag.String("project-prefix", "wp05-retrofit", "project namespace prefix; each fixture item gets its own <prefix>-<question_id> project")
		limit         = flag.Int("limit", 200, "recall limit; must exceed the max haystack session count per item")
		outPath       = flag.String("out", "results/wp05-retrofit/retrofit-bundle.json", "output path for the retrofit bundle JSON")
	)
	flag.Parse()
	applySharedDefaults(flag.CommandLine, serverURL, apiKey)

	if err := run(*dataPath, *serverURL, *apiKey, *projectPrefix, *limit, *outPath); err != nil {
		fmt.Fprintf(os.Stderr, "wp05-retrofit-runner: %v\n", err)
		os.Exit(1)
	}
}

func run(dataPath, serverURL, apiKey, projectPrefix string, limit int, outPath string) error {
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

	harnessSHA, err := gitShortSHA(ctx)
	if err != nil {
		return fmt.Errorf("resolve harness SHA: %w", err)
	}
	runID := "wp05-retrofit-" + time.Now().UTC().Format("20060102T150405Z")
	itemSet := fmt.Sprintf("%s-develop-n%d", strings.TrimSuffix(filepath.Base(dataPath), filepath.Ext(dataPath)), len(items))
	provenance := wp05retrofit.Provenance{
		GoldVersion:   filepath.Base(dataPath),
		ScorerVersion: "wp05-retrofit-v1",
		FeatureFlags:  []string{"layer_b_retrofit"},
		System:        wp05retrofit.SystemName,
		ItemSet:       itemSet,
		RunID:         runID,
		HarnessSHA:    harnessSHA,
	}

	log.Printf("wp05-retrofit-runner: starting %d fixture item(s), project-prefix=%s limit=%d url=%s",
		len(items), projectPrefix, limit, serverURL)

	bundle, err := wp05retrofit.Run(ctx, client, items, wp05retrofit.Config{
		ProjectPrefix:      projectPrefix,
		Limit:              limit,
		ProvenanceTemplate: provenance,
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
	if !flagWasProvided(fs, "url") {
		*serverURL = defaultServerURL()
	}
	if !flagWasProvided(fs, "api-key") {
		*apiKey = defaultAPIKey()
	}
}

func flagWasProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

func defaultAPIKey() string {
	_, token := mcpDefaults()
	return envOr("ENGRAM_API_KEY", token)
}

func defaultServerURL() string {
	url, _ := mcpDefaults()
	return envOr("ENGRAM_URL", url)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// mcpDefaults mirrors cmd/longmemeval's MCP default discovery.
func mcpDefaults() (url, token string) {
	url = "http://localhost:8788"
	home, err := os.UserHomeDir()
	if err != nil {
		return url, ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "mcp_servers.json"))
	if err != nil {
		return url, ""
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return url, ""
	}
	for name, srv := range cfg.McpServers {
		if name != "engram" {
			continue
		}
		srvURL := srv.URL
		if u, err := neturl.Parse(srvURL); err == nil {
			u.Path = strings.TrimSuffix(u.Path, "/sse")
			u.RawQuery = ""
			srvURL = u.String()
		}
		if srvURL != "" {
			url = srvURL
		}
		if auth := srv.Headers["Authorization"]; len(auth) > 7 {
			token = auth[7:]
		}
		return url, token
	}
	return url, token
}

func gitShortSHA(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
