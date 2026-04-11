// Command engram-setup configures the local MCP client (Claude Code) to connect to a
// running engram server.
//
// It calls the unauthenticated /setup-token endpoint on the engram server, retrieves the
// current bearer token, and writes the mcpServers.engram block in ~/.claude.json.
//
// Usage:
//
//	go run ./cmd/engram-setup              # configure with defaults
//	go run ./cmd/engram-setup --dry-run    # preview changes without writing
//	go run ./cmd/engram-setup --port 9000  # non-default port
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "engram-setup: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 8788, "engram server port")
	name := flag.String("name", "engram", "MCP server name to write in ~/.claude.json")
	dryRun := flag.Bool("dry-run", false, "print the MCP config diff without writing ~/.claude.json")
	configPath := flag.String("config", "", "path to Claude config file (default: ~/.claude.json)")
	flag.Parse()

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

	// 3. Locate the Claude config file.
	cfgPath := *configPath
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		cfgPath = filepath.Join(home, ".claude.json")
	}

	// 4. Read existing config (create skeleton if absent).
	raw, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", cfgPath, err)
	}
	var cfg map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("parse %s: %w\n\nFix the JSON and re-run.", cfgPath, err)
		}
	}
	if cfg == nil {
		cfg = make(map[string]json.RawMessage)
	}

	// 5. Merge mcpServers.{name} block — preserve all other MCP servers.
	var mcpServers map[string]json.RawMessage
	if raw, ok := cfg["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &mcpServers); err != nil {
			return fmt.Errorf("parse mcpServers: %w", err)
		}
	}
	if mcpServers == nil {
		mcpServers = make(map[string]json.RawMessage)
	}

	newEntry := map[string]interface{}{
		"type": "sse",
		"url":  setup.Endpoint,
		"headers": map[string]string{
			"Authorization": "Bearer " + setup.Token,
		},
	}
	entryJSON, err := json.MarshalIndent(newEntry, "    ", "  ")
	if err != nil {
		return fmt.Errorf("marshal MCP entry: %w", err)
	}

	existing, alreadyPresent := mcpServers[*name]

	if *dryRun {
		if alreadyPresent {
			fmt.Printf("~ mcpServers.%s (update)\n  was: %s\n  now: %s\n",
				*name, string(existing), string(entryJSON))
		} else {
			fmt.Printf("+ mcpServers.%s (add)\n  %s\n", *name, string(entryJSON))
		}
		fmt.Println("\n(dry-run: no changes written)")
		return nil
	}

	mcpServers[*name] = json.RawMessage(entryJSON)

	mcpRaw, err := json.Marshal(mcpServers)
	if err != nil {
		return fmt.Errorf("marshal mcpServers: %w", err)
	}
	cfg["mcpServers"] = json.RawMessage(mcpRaw)

	// 6. Write back with indentation.
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, append(out, '\n'), 0600); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}

	action := "added"
	if alreadyPresent {
		action = "updated"
	}
	fmt.Printf("✓ engram MCP server %s in %s\n", action, cfgPath)
	fmt.Printf("  endpoint: %s\n", setup.Endpoint)
	fmt.Printf("  token:    %s...%s\n", setup.Token[:8], setup.Token[len(setup.Token)-4:])
	fmt.Println("\nNext step: run /mcp in Claude Code to connect.")
	return nil
}

type setupResponse struct {
	Token    string `json:"token"`
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
}

func healthCheck(base string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func fetchSetupToken(base string) (*setupResponse, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/setup-token")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
