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
)

const usageText = `Usage: starter [subcommand]

Subcommands:
  server    Start the engram MCP server
  migrate   Run database migrations only
  setup     Configure the MCP client
  health    Check /health endpoint and exit 0 (ok) or 1 (error)

Starter fetches secrets from Infisical (or uses ENGRAM_API_KEY directly) and
exec-replaces itself with /engram.
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
		if os.Getenv("ENGRAM_API_KEY") == "" {
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

	apiKey, err := getSecret(ctx, domain, token, projectID, env, secretPath, "ENGRAM_API_KEY")
	if err != nil {
		fatalf("fetch ENGRAM_API_KEY: %v", err)
	}

	if err := os.Setenv("ENGRAM_API_KEY", apiKey); err != nil {
		fatalf("setenv ENGRAM_API_KEY: %v", err)
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

// runHealth probes the engram /health endpoint on localhost and exits 0 if the
// response is HTTP 200, or 1 on any error or non-200 status. Used as the
// Docker HEALTHCHECK command in distroless images that have no shell or wget.
func runHealth() {
	port := "8788"
	client := &http.Client{Timeout: 4 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/health", nil)
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

// execEngram validates subcommand arguments and exec-replaces the current
// process with /engram using the supplied environment.
func execEngram(args []string, cleanEnv []string) {
	allowed := map[string]bool{"server": true, "migrate": true, "setup": true}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && !allowed[arg] {
			fatalf("unknown subcommand %q — allowed: server, migrate, setup", arg)
		}
	}
	argv := append([]string{"/engram"}, args...)
	if err := syscall.Exec("/engram", argv, cleanEnv); err != nil { // nosemgrep: go.lang.security.audit.dangerous-syscall-exec.dangerous-syscall-exec -- argv validated against allowlist above
		fatalf("exec /engram: %v", err)
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
