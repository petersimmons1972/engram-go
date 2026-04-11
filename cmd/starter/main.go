// Command starter authenticates to Infisical via machine identity, injects
// secrets into the process environment, then exec-replaces itself with engram.
// It is the container ENTRYPOINT — no shell, no external HTTP client required.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"syscall"
	"time"
)

// infisicalDomainRE accepts only https:// URLs with a safe hostname.
// This prevents INFISICAL_DOMAIN from being set to an attacker-controlled host
// that would redirect machine-identity authentication (#135).
var infisicalDomainRE = regexp.MustCompile(`^https://[a-zA-Z0-9][a-zA-Z0-9\-\.]+$`)

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
	clientID := mustEnv("INFISICAL_CLIENT_ID")
	clientSecret := mustEnv("INFISICAL_CLIENT_SECRET")
	domain := envOr("INFISICAL_DOMAIN", "https://infisical.petersimmons.com")
	projectID := envOr("INFISICAL_PROJECT_ID", "f49c5b01-4bd1-4883-afbd-51c1fef53a2f")
	env := envOr("INFISICAL_ENV", "prod")
	secretPath := envOr("INFISICAL_SECRET_PATH", "/engram")

	// Validate Infisical domain to prevent supply-chain redirect attacks (#135).
	if !infisicalDomainRE.MatchString(domain) {
		fatalf("INFISICAL_DOMAIN %q is invalid — must be https://<hostname> with no path or credentials", domain)
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

	// Strip Infisical machine-identity credentials from the environment before
	// exec-replacing ourselves with engram. The engram process has no need for
	// these credentials — keeping them in /proc/PID/environ leaks them to any
	// process that can read /proc (#138).
	cleanEnv := filteredEnv("INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET")

	argv := append([]string{"/engram"}, os.Args[1:]...)
	if err := syscall.Exec("/engram", argv, cleanEnv); err != nil {
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
