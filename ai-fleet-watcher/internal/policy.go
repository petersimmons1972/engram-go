package internal

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const policyFile = "/data/policy.json"

// mtlsClient returns an http.Client configured with mTLS client certificates
// from /run/secrets/ (Docker secrets mount). Falls back to default transport
// when certs are absent — allowing unauthenticated controller access in dev.
//
// Server cert verification uses the system CA pool (controller uses Let's Encrypt).
// The custom ai-fleet-ca in /run/secrets/ca.crt is for client cert issuance only —
// it is NOT used as a RootCA here.
func mtlsClient() *http.Client {
	const (
		certFile = "/run/secrets/client.crt"
		keyFile  = "/run/secrets/client.key"
	)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		// Certs not available — plain client (dev mode).
		return &http.Client{Timeout: 10 * time.Second}
	}
	tlsCfg := &tls.Config{
		// RootCAs intentionally nil — use system pool (Let's Encrypt trusted).
		Certificates: []tls.Certificate{cert},
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
}

// FetchPolicy calls GET /policy/{hostname} on the controller.
func FetchPolicy(ctx context.Context, controllerURL, hostname string) (*Policy, error) {
	// Controller expects the bare hostname (strip any port).
	host := strings.SplitN(hostname, ":", 2)[0]
	url := strings.TrimRight(controllerURL, "/") + "/policy/" + host

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := mtlsClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch policy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // host not registered yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("controller returned %d", resp.StatusCode)
	}

	var p Policy
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	return &p, nil
}

// SavePolicy persists policy to disk for offline recovery.
func SavePolicy(p *Policy) {
	if err := os.MkdirAll(filepath.Dir(policyFile), 0o755); err != nil {
		slog.Warn("save policy mkdir", "err", err)
		return
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(policyFile, b, 0o644); err != nil {
		slog.Warn("save policy write", "err", err)
	}
}

// LoadLastPolicy reads the last persisted policy from disk.
func LoadLastPolicy() *Policy {
	b, err := os.ReadFile(policyFile)
	if err != nil {
		return nil
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	slog.Info("loaded last-known policy", "version", p.PolicyVersion)
	return &p
}

// PostStatus reports container health to the controller.
func PostStatus(ctx context.Context, controllerURL string, report StatusReport) {
	url := strings.TrimRight(controllerURL, "/") + "/status/" + report.Hostname
	b, _ := json.Marshal(report)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader(string(b)))
	if err != nil {
		slog.Warn("post status build req", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := mtlsClient()
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("post status", "err", err)
		return
	}
	resp.Body.Close()
}
