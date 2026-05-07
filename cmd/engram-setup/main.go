// Command engram-setup configures the local MCP client (Claude Code) to connect to a
// running engram server.
//
// Primary path: calls /setup-token (requires Bearer auth since #540) to retrieve the
// current token and writes the mcpServers.engram block in the Claude Code config.
//
// Fallback path (#614, #616): when /setup-token returns 401 (bootstrap scenario where
// no valid token exists yet), engram-setup reads the key from disk in priority order:
//  1. ~/.config/engram/api_key — backup written by `make init`, matches Infisical secret
//  2. ENGRAM_API_KEY in ~/projects/engram-go/.env — lower trust, may diverge from Infisical
//
// Each candidate key is probed against /quick-recall before being written, so a stale
// or wrong key on disk never silently corrupts the config.
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
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)".
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
	offline := flag.Bool("offline", false, "skip /setup-token call (useful when server is rate-limited during setup)")
	format := flag.String("format", "text", "output format: text or json")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("engram-setup %s\n", Version)
		os.Exit(0)
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", *port)

	// 1. Verify server is reachable (unless --offline).
	if !*offline {
		if err := healthCheck(base); err != nil {
			return fmt.Errorf(
				"engram server not reachable at %s/health\n\n"+
					"  Is it running?  →  docker compose up -d\n"+
					"  Check logs?     →  make logs\n\n"+
					"  (original error: %w)", base, err)
		}

		// 2. Fetch the current bearer token.
		// Falls back to key sources on disk when /setup-token returns 401 (#614, #616).
		// /starter injects ENGRAM_API_KEY from Infisical at runtime; ~/.config/engram/api_key
		// is the backup written by `make init` and is the most reliable local copy.
		setup, setupErr := fetchSetupToken(base)
		if setupErr != nil {
			home, _ := os.UserHomeDir()
			httpClient := &http.Client{Timeout: 5 * time.Second}
			// Each candidate produces a raw key string; probeAuth validates it.
			type candidate struct {
				label string
				key   func() string
			}
			candidates := []candidate{
				{
					label: "~/.config/engram/api_key",
					key: func() string {
						k, _ := tryKeyFromDisk(
							filepath.Join(home, ".config", "engram", "api_key"),
							base, httpClient)
						return k
					},
				},
				{
					label: "~/projects/engram-go/.env (ENGRAM_API_KEY)",
					key: func() string {
						raw := readKeyFromEnvFile(filepath.Join(home, "projects", "engram-go", ".env"))
						if raw == "" || len(raw) < 12 {
							return ""
						}
						ok, _ := probeAuth(raw, base, httpClient)
						if !ok {
							return ""
						}
						return raw
					},
				},
			}
			for _, c := range candidates {
				if key := c.key(); key != "" {
					fmt.Fprintf(os.Stderr,
						"engram-setup: /setup-token unavailable (%v) — recovered key from %s\n",
						setupErr, c.label)
					stub := &setupResponse{Token: key, Endpoint: fmt.Sprintf("http://127.0.0.1:%d/sse", *port)}
					return configureWithSetup(base, *name, *dryRun, *format, stub)
				}
			}
			return fmt.Errorf("fetch /setup-token: %w\n\n"+
				"  Disk fallbacks (~/.config/engram/api_key, .env) also failed.\n"+
				"  Ensure ENGRAM_API_KEY is set correctly, then run: make init && make restart", setupErr)
		}
		return configureWithSetup(base, *name, *dryRun, *format, setup)
	}

	// --offline mode: stub output without server calls
	// User can use --offline when the server is rate-limited (#589)
	// In offline mode, we cannot fetch a real token, so generate a stub (#589)
	stubSetup := &setupResponse{
		Token:    "stub-offline-token-" + fmt.Sprintf("%d", time.Now().Unix()),
		Endpoint: fmt.Sprintf("http://127.0.0.1:%d/sse", *port),
		Name:     *name,
	}
	return configureWithSetup(base, *name, *dryRun, *format, stubSetup)
}

func configureWithSetup(base, name string, dryRun bool, format string, setup *setupResponse) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home directory: %w", err)
	}

	// Claude Code reads MCP servers from both of these files — keep both in sync.
	targets := []string{
		filepath.Join(home, ".claude", "mcp_servers.json"), // primary: Claude Code live config
		filepath.Join(home, ".claude.json"),                // secondary: Claude Code user settings
	}

	newEntry := map[string]interface{}{
		"type": "sse",
		"url":  setup.Endpoint,
		"headers": map[string]string{
			"Authorization": "Bearer " + setup.Token,
		},
	}

	if dryRun {
		// Redact token in dry-run output
		redactedToken := redactToken(setup.Token)
		displayEntry := map[string]interface{}{
			"type": "sse",
			"url":  setup.Endpoint,
			"headers": map[string]string{
				"Authorization": "Bearer " + redactedToken,
			},
		}

		if format == "json" {
			// JSON format for --dry-run --format=json
			output := map[string]interface{}{
				"status":   "dry-run",
				"endpoint": setup.Endpoint,
				"token":    redactedToken,
				"would_write": map[string]interface{}{
					"targets": targets,
					"entry":   displayEntry,
				},
			}
			outJSON, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(outJSON))
		} else {
			// Text format for --dry-run (default)
			displayJSON, _ := json.MarshalIndent(displayEntry, "    ", "  ")
			fmt.Printf("DRY RUN — would write mcpServers.%s to:\n  %s\n  %s\n\n",
				name, targets[0], targets[1])
			fmt.Printf("  entry: %s\n\n(no changes written)\n", string(displayJSON))
		}
		return nil
	}

	var updated []string
	for _, cfgPath := range targets {
		action, err := updateMCPConfig(cfgPath, name, newEntry)
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

	// Format output based on --format flag
	if format == "json" {
		output := map[string]interface{}{
			"status":   "configured",
			"endpoint": setup.Endpoint,
			"token":    redactToken(setup.Token),
			"written":  updated,
		}
		outJSON, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(outJSON))
	} else {
		fmt.Printf("engram configured.\n")
		fmt.Printf("  endpoint: %s\n", setup.Endpoint)
		fmt.Printf("  token:    %s\n", redactToken(setup.Token))
		for _, u := range updated {
			fmt.Printf("  wrote:    %s\n", u)
		}
		fmt.Println("Run /mcp in Claude Code to reconnect.")
	}
	return nil
}

func redactToken(token string) string {
	if len(token) < 12 {
		return "***"
	}
	return token[:8] + "..." + token[len(token)-4:]
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
	return healthCheckWithClient(base, &http.Client{Timeout: 5 * time.Second})
}

func healthCheckWithClient(base string, client *http.Client) error {
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

// tryKeyFromDisk reads a bearer key from a raw-key file (one key per file, no
// key=value format), trims whitespace, and probes /quick-recall to confirm the
// key authenticates. Returns the key on success, "" if the file is absent or the
// key fails auth, or an error only for unexpected failures (#614, #616).
func tryKeyFromDisk(path, base string, client *http.Client) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil // absent or unreadable — caller tries next source
	}
	key := strings.TrimSpace(string(raw))
	if len(key) < 12 {
		return "", nil // too short to be a valid token
	}
	ok, err := probeAuth(key, base, client)
	if err != nil || !ok {
		return "", nil
	}
	return key, nil
}

// readKeyFromEnvFile reads the value of ENGRAM_API_KEY from a .env-format file.
// Returns the last matching value, or "" if the file is absent or the key is not set.
func readKeyFromEnvFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck
	var last string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "ENGRAM_API_KEY=") {
			continue
		}
		val := strings.TrimPrefix(line, "ENGRAM_API_KEY=")
		if val != "" {
			last = val
		}
	}
	return last
}

// probeAuth sends a /quick-recall POST with the given key and returns whether
// the server accepted it. Returns (false, nil) on 401; (false, err) on transport
// failure; (true, nil) on any 2xx or 5xx (token accepted, backend may have erred).
func probeAuth(key, base string, client *http.Client) (bool, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		base+"/quick-recall",
		strings.NewReader(`{"query":"auth-check","project":"global","limit":1}`))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, nil // network failure — treat as auth miss, not hard error
	}
	resp.Body.Close() //nolint:errcheck
	return resp.StatusCode != http.StatusUnauthorized, nil
}

func fetchSetupToken(base string) (*setupResponse, error) {
	return fetchSetupTokenWithClient(base, &http.Client{Timeout: 5 * time.Second})
}

func fetchSetupTokenWithClient(base string, client *http.Client) (*setupResponse, error) {
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
