// Command starter authenticates to Infisical via machine identity, injects
// secrets into the process environment, then exec-replaces itself with engram.
// It is the container ENTRYPOINT — no shell, no external HTTP client required.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
)

func main() {
	clientID := mustEnv("INFISICAL_CLIENT_ID")
	clientSecret := mustEnv("INFISICAL_CLIENT_SECRET")
	domain := envOr("INFISICAL_DOMAIN", "https://infisical.petersimmons.com")
	projectID := envOr("INFISICAL_PROJECT_ID", "f49c5b01-4bd1-4883-afbd-51c1fef53a2f")
	env := envOr("INFISICAL_ENV", "prod")
	secretPath := envOr("INFISICAL_SECRET_PATH", "/engram")

	token, err := getAccessToken(domain, clientID, clientSecret)
	if err != nil {
		fatalf("infisical auth: %v", err)
	}

	apiKey, err := getSecret(domain, token, projectID, env, secretPath, "ENGRAM_API_KEY")
	if err != nil {
		fatalf("fetch ENGRAM_API_KEY: %v", err)
	}

	if err := os.Setenv("ENGRAM_API_KEY", apiKey); err != nil {
		fatalf("setenv: %v", err)
	}

	argv := append([]string{"/engram"}, os.Args[1:]...)
	if err := syscall.Exec("/engram", argv, os.Environ()); err != nil {
		fatalf("exec /engram: %v", err)
	}
}

func getAccessToken(domain, clientID, clientSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
	})
	resp, err := http.Post(domain+"/api/v1/auth/universal-auth/login", "application/json", bytes.NewReader(body))
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

func getSecret(domain, token, projectID, environment, secretPath, name string) (string, error) {
	url := fmt.Sprintf("%s/api/v3/secrets/raw/%s?workspaceId=%s&environment=%s&secretPath=%s",
		domain, name, projectID, environment, secretPath)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
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
