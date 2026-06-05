// Command starter authenticates to Infisical via machine identity, injects
// secrets into the process environment, then exec-replaces itself with engram.
// It is the container ENTRYPOINT — no shell, no external HTTP client required.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/petersimmons1972/engram/internal/config"
)

const usageText = `Usage: starter [subcommand] [options]

Subcommands:
  server    Start the engram MCP server
            Use 'starter server --help' for options
  migrate   Run database migrations only
            Use 'starter migrate --help' for options
  health    Check /health endpoint and exit 0 (ok) or 1 (error)
            Use 'starter health --help' for options
  help      Show this help message

Starter fetches secrets from Infisical (or uses ENGRAM_API_KEY directly) and
exec-replaces itself with /engram.
For client MCP configuration, run engram-setup on the host machine.

Options:
  -h, --help    Show this help message
`

const starterServerHelpText = `Usage: starter server [options]

Fetch container secrets when configured, then exec /engram with server options.
For the full server flag list, run engram server --help in a developer checkout
or /engram server --help inside the container image.
`

const starterMigrateHelpText = `Usage: starter migrate [options]

Fetch database credentials when Infisical is configured, then exec
/engram migrate to run schema migrations and exit without starting the server.
For local developer runs, use go run ./cmd/engram migrate --help.
`

const starterHealthHelpText = `Usage: starter health

Probe the local /health endpoint and exit 0 when healthy or 1 on error.
`

func printUsage() {
	_, _ = fmt.Fprint(os.Stdout, usageText)
}

// patchDatabaseURLPassword replaces the password in a PostgreSQL DSN with
// the supplied password. Returns the original dsn unchanged on any parse error.
func patchDatabaseURLPassword(dsn, password string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	u.User = url.UserPassword(u.User.Username(), password)
	return u.String()
}

// infisicalDomainRE accepts only https:// URLs with a proper FQDN hostname
// (at least two dot-separated labels, e.g. infisical.example.com).
// The FQDN requirement blocks localhost and single-label hostnames.
// Each label: starts with alphanumeric, may contain hyphens.
// Raw IP addresses (which also satisfy the dot rule) are blocked by
// isValidInfisicalDomain using net.ParseIP.
var infisicalDomainRE = regexp.MustCompile(`^https://[a-zA-Z0-9][a-zA-Z0-9\-]*(\.[a-zA-Z0-9][a-zA-Z0-9\-]*)+$`)

// isValidInfisicalDomain returns true iff domain is a safe Infisical base URL.
// It combines the FQDN regex with an IP-address rejection to prevent SSRF (#135).
func isValidInfisicalDomain(domain string) bool {
	if !infisicalDomainRE.MatchString(domain) {
		return false
	}
	// Strip the scheme to get the raw host, then reject raw IP literals.
	host := domain[len("https://"):]
	return net.ParseIP(host) == nil
}

// infisicalHTTPClient has explicit timeouts so an unreachable Infisical
// host does not hang the container startup indefinitely (#137).
var infisicalHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	},
}

func main() {
	help := flag.Bool("h", false, "show help and exit")
	flag.BoolVar(help, "help", false, "show help and exit")
	flag.Usage = func() {
		printUsage()
	}
	flag.Parse()

	// Handle "help" subcommand and -h/--help flags — all exit 0.
	args := flag.Args()
	if *help || (len(args) > 0 && args[0] == "help") {
		printUsage()
		os.Exit(0)
	}
	if helpText, ok := starterSubcommandHelp(args); ok {
		_, _ = fmt.Fprint(os.Stdout, helpText)
		os.Exit(0)
	}

	// Health subcommand: probe the local /health endpoint and exit 0 (ok) or 1 (error).
	// Runs before the Infisical flow — no secrets required.
	if len(args) > 0 && args[0] == "health" {
		runHealth()
		return
	}

	clientID := os.Getenv("INFISICAL_CLIENT_ID")

	// If INFISICAL_CLIENT_ID is absent, skip the Infisical flow entirely.
	// The environment must already contain ENGRAM_API_KEY in this case.
	if clientID == "" {
		if starterRequiresAPIKey(args) && os.Getenv("ENGRAM_API_KEY") == "" {
			fatalf("no secret source configured: set INFISICAL_CLIENT_ID (+ INFISICAL_CLIENT_SECRET) to fetch secrets from Infisical, or set ENGRAM_API_KEY directly in the environment")
		}
		// ENGRAM_API_KEY is already set — exec directly without touching the env.
		execEngram(args, filteredEnv())
		return
	}

	clientSecret := mustEnv("INFISICAL_CLIENT_SECRET")
	domain := envOr("INFISICAL_DOMAIN", "https://infisical.petersimmons.com")
	projectID := mustEnv("INFISICAL_PROJECT_ID")
	env := envOr("INFISICAL_ENV", "prod")
	secretPath := envOr("INFISICAL_SECRET_PATH", "/apps/engram")

	// Validate Infisical domain to prevent supply-chain redirect attacks (#135).
	if !isValidInfisicalDomain(domain) {
		fatalf("INFISICAL_DOMAIN %q is invalid — must be https://<fqdn> with no path, credentials, or raw IP", domain)
	}

	ctx := context.Background()
	token, err := getAccessToken(ctx, domain, clientID, clientSecret)
	if err != nil {
		fatalf("infisical auth: %v", err)
	}

	if starterRequiresAPIKey(args) {
		apiKey, err := getSecret(ctx, domain, token, projectID, env, secretPath, "ENGRAM_API_KEY")
		if err != nil {
			fatalf("fetch ENGRAM_API_KEY: %v", err)
		}

		if err := os.Setenv("ENGRAM_API_KEY", apiKey); err != nil {
			fatalf("setenv ENGRAM_API_KEY: %v", err)
		}
	}

	// Fetch POSTGRES_PASSWORD and patch DATABASE_URL so the correct password is
	// injected at runtime rather than relying on the .env file on disk.
	pgPassword, err := getSecret(ctx, domain, token, projectID, env, secretPath, "POSTGRES_PASSWORD")
	if err != nil {
		fatalf("fetch POSTGRES_PASSWORD: %v", err)
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		patched := patchDatabaseURLPassword(dbURL, pgPassword)
		if err := os.Setenv("DATABASE_URL", patched); err != nil {
			fatalf("setenv DATABASE_URL: %v", err)
		}
	}

	// Strip Infisical machine-identity credentials and POSTGRES_PASSWORD from the
	// environment before exec-replacing ourselves with engram. The engram process
	// has no need for these credentials — keeping them in /proc/PID/environ leaks
	// them to any process that can read /proc (#138, #139).
	execEngram(args, filteredEnv("INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET", "POSTGRES_PASSWORD"))
}

func starterSubcommandHelp(args []string) (string, bool) {
	if len(args) < 2 || !isHelpArg(args[1]) {
		return "", false
	}
	switch args[0] {
	case "server":
		return starterServerHelpText, true
	case "migrate":
		return starterMigrateHelpText, true
	case "health":
		return starterHealthHelpText, true
	default:
		return "", false
	}
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func starterRequiresAPIKey(args []string) bool {
	return len(args) == 0 || args[0] != "migrate"
}

// runHealth probes the engram /health endpoint on localhost and exits 0 if the
// response is HTTP 200, or 1 on any error or non-200 status. Used as the
// Docker HEALTHCHECK command in distroless images that have no shell or wget.
//
// The port is read from ENGRAM_PORT (canonical default: config.DefaultPort).
// This ensures the healthcheck always targets the same port the binary binds to,
// preventing false-positives when ENGRAM_PORT is overridden. (#729).
func runHealth() {
	port := config.PortOrDefault()
	client := &http.Client{Timeout: 4 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/health", port), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health: %v\n", err)
		os.Exit(1)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health: %v\n", err)
		os.Exit(1)
	}
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "health: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

var execCommand = func(path string, argv []string, env []string) error {
	return syscall.Exec(path, argv, env)
}

// execEngram resolves the subcommand (server, migrate), then exec-replaces
// the current process with /engram using the supplied environment.
//
// For server, the subcommand is consumed (omitted from argv) so /engram receives
// only server flags. For migrate the subcommand is preserved so engram can route
// the one-shot migration operation by name.
func execEngram(args []string, cleanEnv []string) {
	path, argv, err := starterExecPlan(args)
	if err != nil {
		fatalf("%v", err)
	}

	if err := execCommand(path, argv, cleanEnv); err != nil { // nosemgrep: go.lang.security.audit.dangerous-syscall-exec.dangerous-syscall-exec -- argv validated against allowlist above
		fatalf("exec /engram: %v", err)
	}
}

func starterExecPlan(args []string) (string, []string, error) {
	subcommand, passthrough, err := resolveStarterSubcommand(args)
	if err != nil {
		return "", nil, err
	}
	argv := []string{"/engram"}
	if subcommand != "server" {
		argv = append(argv, subcommand)
	}
	argv = append(argv, passthrough...)
	return "/engram", argv, nil
}

// resolveStarterSubcommand handles explicit subcommands and flag-prefixed args.
// `server` is the default when args are empty or begin with '-'.
func resolveStarterSubcommand(args []string) (subcommand string, passthrough []string, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "server", args, nil
	}

	switch args[0] {
	case "server", "migrate":
		return args[0], args[1:], nil
	default:
		return "", nil, fmt.Errorf("unknown subcommand %q — allowed: server, migrate", args[0])
	}
}

// filteredEnv returns os.Environ() with the named keys removed.
func filteredEnv(removeKeys ...string) []string {
	drop := make(map[string]bool, len(removeKeys))
	for _, k := range removeKeys {
		drop[k] = true
	}
	all := os.Environ()
	out := make([]string, 0, len(all))
	for _, kv := range all {
		key := kv
		if i := len(kv); i > 0 {
			for j := 0; j < len(kv); j++ {
				if kv[j] == '=' {
					key = kv[:j]
					break
				}
			}
		}
		if !drop[key] {
			out = append(out, kv)
		}
	}
	return out
}

func getAccessToken(ctx context.Context, domain, clientID, clientSecret string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("marshal auth request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		domain+"/api/v1/auth/universal-auth/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := infisicalHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, raw)
	}
	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return result.AccessToken, nil
}

func getSecret(ctx context.Context, domain, token, projectID, environment, secretPath, name string) (string, error) {
	// URL-encode each query parameter individually to prevent injection (#146).
	q := url.Values{}
	q.Set("workspaceId", projectID)
	q.Set("environment", environment)
	q.Set("secretPath", secretPath)
	rawURL := domain + "/api/v3/secrets/raw/" + url.PathEscape(name) + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build secret request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := infisicalHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, raw)
	}
	var result struct {
		Secret struct {
			SecretValue string `json:"secretValue"`
		} `json:"secret"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if result.Secret.SecretValue == "" {
		return "", fmt.Errorf("empty secret value for %s", name)
	}
	return result.Secret.SecretValue, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "starter: "+format+"\n", args...)
	os.Exit(1)
}
