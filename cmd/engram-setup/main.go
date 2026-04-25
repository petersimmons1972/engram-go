// Command engram-setup configures the local MCP client (Claude Code) to connect to a
// running engram server.
//
// It calls the unauthenticated /setup-token endpoint on the engram server, retrieves the
// current bearer token, and writes the mcpServers.engram block in the Claude Code config.
//
// Claude Code reads MCP servers from two files:
//   - ~/.claude/mcp_servers.json  — primary (live config, read each session)
//   - ~/.claude.json              — secondary (user settings, also read at startup)
//
// engram-setup writes both so the token stays fresh regardless of which file Claude
// Code happens to use in a given version.
//
// Usage:
//
//	go run ./cmd/engram-setup              # configure with defaults
//	go run ./cmd/engram-setup --dry-run    # preview changes without writing
//	go run ./cmd/engram-setup --port 9000  # non-default port
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)"
var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "engram-setup: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 8788, "engram server port")
	name := flag.String("name", "engram", "MCP server name to write in Claude config files")
	dryRun := flag.Bool("dry-run", false, "print the MCP config diff without writing any files")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("engram-setup %s\n", Version)
		os.Exit(0)
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", *port)

	// 1. Verify server is reachable.
	if err := healthCheck(base); err != nil {
		return fmt.Errorf(
			"engram server not reachable at %s/health\n\n"+
				"  Is it running?  →  docker compose up -d\n"+
				"  Check logs?     →  make logs\n\n"+
				"  (original error: %w)", base, err)
	}

	// 2. Fetch the current bearer token.
	setup, err := fetchSetupToken(base)
	if err != nil {
		return fmt.Errorf("fetch /setup-token: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home directory: %w", err)
	}

	// Claude Code reads MCP servers from both of these files — keep both in sync.
	targets := []string{
		filepath.Join(home, ".claude", "mcp_servers.json"), // primary: Claude Code live config
		filepath.Join(home, ".claude.json"),                 // secondary: Claude Code user settings
	}

	newEntry := map[string]interface{}{
		"type": "sse",
		"url":  setup.Endpoint,
		"headers": map[string]string{
			"Authorization": "Bearer " + setup.Token,
		},
	}

	if *dryRun {
		entryJSON, _ := json.MarshalIndent(newEntry, "    ", "  ")
		fmt.Printf("DRY RUN — would write mcpServers.%s to:\n  %s\n  %s\n\n",
			*name, targets[0], targets[1])
		fmt.Printf("  entry: %s\n\n(no changes written)\n", string(entryJSON))
		return nil
	}

	var updated []string
	for _, cfgPath := range targets {
		action, err := updateMCPConfig(cfgPath, *name, newEntry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engram-setup: warning: could not update %s: %v\n", cfgPath, err)
			continue
		}
		if action != "" {
			updated = append(updated, fmt.Sprintf("%s (%s)", cfgPath, action))
		}
	}

	if len(updated) == 0 {
		return fmt.Errorf("failed to update any config file")
	}

	fmt.Printf("engram configured.\n")
	fmt.Printf("  endpoint: %s\n", setup.Endpoint)
	fmt.Printf("  token:    %s...%s\n", setup.Token[:8], setup.Token[len(setup.Token)-4:])
	for _, u := range updated {
		fmt.Printf("  wrote:    %s\n", u)
	}
	fmt.Println("Run /mcp in Claude Code to reconnect.")
	return nil
}

// updateMCPConfig reads cfgPath, upserts mcpServers[name]=entry, and writes it back.
// Returns the action taken ("added" or "updated"), or "" if the file was skipped.
// For ~/.claude.json specifically: only writes if the file exists (it's the main user
// settings file — we don't want to clobber it if it's absent or has no mcpServers yet).
func updateMCPConfig(cfgPath, name string, entry map[string]interface{}) (string, error) {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			// ~/.claude.json: skip if absent (we don't create user settings from scratch).
			// ~/.claude/mcp_servers.json: create it — it's a dedicated MCP config file.
			if filepath.Base(cfgPath) == ".claude.json" {
				return "", nil // skip
			}
			raw = []byte(`{"mcpServers":{}}`)
		} else {
			return "", fmt.Errorf("read: %w", err)
		}
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse JSON: %w", err)
	}
	if cfg == nil {
		cfg = make(map[string]json.RawMessage)
	}

	// For ~/.claude.json: only update the mcpServers block if it already exists.
	// Never add a new mcpServers key to the user settings file unprompted.
	if filepath.Base(cfgPath) == ".claude.json" {
		if _, hasMCP := cfg["mcpServers"]; !hasMCP {
			return "", nil // skip
		}
	}

	var mcpServers map[string]json.RawMessage
	if existing, ok := cfg["mcpServers"]; ok {
		if err := json.Unmarshal(existing, &mcpServers); err != nil {
			return "", fmt.Errorf("parse mcpServers: %w", err)
		}
	}
	if mcpServers == nil {
		mcpServers = make(map[string]json.RawMessage)
	}

	_, alreadyPresent := mcpServers[name]
	entryJSON, err := json.MarshalIndent(entry, "    ", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal entry: %w", err)
	}
	mcpServers[name] = json.RawMessage(entryJSON)

	mcpRaw, err := json.Marshal(mcpServers)
	if err != nil {
		return "", fmt.Errorf("marshal mcpServers: %w", err)
	}
	cfg["mcpServers"] = json.RawMessage(mcpRaw)

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(cfgPath, append(out, '\n'), 0600); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	action := "added"
	if alreadyPresent {
		action = "updated"
	}
	return action, nil
}

type setupResponse struct {
	Token    string `json:"token"`
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
}

func healthCheck(base string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func fetchSetupToken(base string) (*setupResponse, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/setup-token", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("/setup-token is localhost-only — run engram-setup directly on the host machine, not inside the container")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var s setupResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if s.Token == "" {
		return nil, fmt.Errorf("server returned empty token")
	}
	if len(s.Token) < 12 {
		return nil, fmt.Errorf("token too short to safely display")
	}
	return &s, nil
}
