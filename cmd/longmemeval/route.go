package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type routeDiscoverConfig struct {
	FleetURL     string
	OllaURL      string
	Model        string
	Purpose      string
	FleetCert    string
	FleetKey     string
	FleetCA      string
	RequestLimit time.Duration
}

type routeDiscoverResult struct {
	FleetURL    string      `json:"fleet_url"`
	OllaURL     string      `json:"olla_url"`
	LLMBaseURL  string      `json:"llm_url"`
	LLMModel    string      `json:"llm_model"`
	Purpose     string      `json:"purpose"`
	FleetHosts  []fleetHost `json:"fleet_hosts"`
	OllaModels  []string    `json:"olla_models"`
	RunFlagHint []string    `json:"run_flag_hint"`
	ScorerHint  []string    `json:"scorer_flag_hint"`
	Source      string      `json:"source"`
	Validated   bool        `json:"validated"`
}

type fleetHost struct {
	Host       string       `json:"host"`
	Models     []fleetModel `json:"models,omitempty"`
	Containers []fleetModel `json:"containers,omitempty"`
}

type fleetModel struct {
	Name      string `json:"name"`
	Framework string `json:"framework,omitempty"`
	Port      int    `json:"port,omitempty"`
	Status    string `json:"status,omitempty"`
	State     string `json:"state,omitempty"`
}

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type openAIModel struct {
	ID string `json:"id"`
}

func discoverRoute(cfg routeDiscoverConfig) (routeDiscoverResult, error) {
	if cfg.FleetURL == "" {
		return routeDiscoverResult{}, errors.New("--fleet-url is required")
	}
	if cfg.OllaURL == "" {
		return routeDiscoverResult{}, errors.New("--olla-url is required")
	}
	if cfg.Purpose == "" {
		cfg.Purpose = "generation"
	}
	timeout := cfg.RequestLimit
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	fleetClient, err := routeHTTPClient(cfg)
	if err != nil {
		return routeDiscoverResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fleetHosts, err := fetchFleetRegistry(ctx, fleetClient, strings.TrimRight(cfg.FleetURL, "/")+"/registry")
	if err != nil {
		return routeDiscoverResult{}, fmt.Errorf("query AI Flight Controller registry: %w", err)
	}
	ollaModels, err := fetchOllaModels(ctx, http.DefaultClient, strings.TrimRight(cfg.OllaURL, "/")+"/olla/openai/v1/models")
	if err != nil {
		return routeDiscoverResult{}, fmt.Errorf("query Olla models: %w", err)
	}

	selected, err := selectRouteModel(cfg.Model, cfg.Purpose, fleetHosts, ollaModels)
	if err != nil {
		return routeDiscoverResult{}, err
	}
	baseURL := strings.TrimRight(cfg.OllaURL, "/") + "/olla/openai/v1"
	validated := false
	if requiresChatReadiness(cfg.Purpose) {
		if err := validateOllaChatRoute(ctx, http.DefaultClient, baseURL, selected); err != nil {
			return routeDiscoverResult{}, fmt.Errorf("validate Olla chat route for %q: %w", selected, err)
		}
		validated = true
	}
	return routeDiscoverResult{
		FleetURL:   cfg.FleetURL,
		OllaURL:    cfg.OllaURL,
		LLMBaseURL: baseURL,
		LLMModel:   selected,
		Purpose:    cfg.Purpose,
		FleetHosts: fleetHosts,
		OllaModels: ollaModels,
		RunFlagHint: []string{
			"--llm-url", baseURL,
			"--llm-model", selected,
		},
		ScorerHint: []string{
			"--scorer-url", baseURL,
			"--scorer-model", selected,
		},
		Source:    "ai-fleet-registry+olla-openai-models",
		Validated: validated,
	}, nil
}

func routeHTTPClient(cfg routeDiscoverConfig) (*http.Client, error) {
	if cfg.FleetCert == "" && cfg.FleetKey == "" && cfg.FleetCA == "" {
		return http.DefaultClient, nil
	}
	if cfg.FleetCert == "" || cfg.FleetKey == "" {
		return nil, errors.New("--fleet-cert and --fleet-key must be provided together")
	}
	cert, err := tls.LoadX509KeyPair(cfg.FleetCert, cfg.FleetKey)
	if err != nil {
		return nil, fmt.Errorf("load fleet client cert/key: %w", err)
	}
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	if cfg.FleetCA != "" {
		caPEM, err := os.ReadFile(cfg.FleetCA)
		if err != nil {
			return nil, fmt.Errorf("read fleet CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse fleet CA %s", cfg.FleetCA)
		}
		tlsCfg.RootCAs = pool
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}, nil
}

func fetchFleetRegistry(ctx context.Context, client *http.Client, url string) ([]fleetHost, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	var hosts []fleetHost
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, errors.New("registry returned no hosts")
	}
	for i := range hosts {
		if len(hosts[i].Models) == 0 && len(hosts[i].Containers) > 0 {
			hosts[i].Models = hosts[i].Containers
		}
		hosts[i].Containers = nil
	}
	return hosts, nil
}

func fetchOllaModels(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("olla returned no models")
	}
	return models, nil
}

func selectRouteModel(requested, purpose string, hosts []fleetHost, ollaModels []string) (string, error) {
	fleetModels := make(map[string]fleetModel)
	for _, host := range hosts {
		for _, model := range host.Models {
			if model.Name != "" {
				fleetModels[model.Name] = model
			}
		}
	}
	ollaSet := make(map[string]bool, len(ollaModels))
	for _, model := range ollaModels {
		ollaSet[model] = true
	}
	if requested != "" {
		if !ollaSet[requested] {
			return "", fmt.Errorf("requested model %q is absent from Olla model discovery", requested)
		}
		if _, ok := fleetModels[requested]; !ok {
			return "", fmt.Errorf("requested model %q is absent from AI Flight Controller registry", requested)
		}
		if purpose == "generation" || purpose == "scoring" {
			if isEmbeddingRoute(requested, fleetModels[requested].Framework) {
				return "", fmt.Errorf("requested model %q is embedding-like and not compatible with %s", requested, purpose)
			}
		}
		if !isFleetModelReady(fleetModels[requested]) {
			return "", fmt.Errorf("requested model %q is not ready in AI Flight Controller registry", requested)
		}
		return requested, nil
	}
	for _, model := range ollaModels {
		fm, ok := fleetModels[model]
		if !ok {
			continue
		}
		if !isFleetModelReady(fm) {
			continue
		}
		if purpose == "generation" || purpose == "scoring" {
			if isEmbeddingRoute(model, fm.Framework) {
				continue
			}
		}
		return model, nil
	}
	return "", errors.New("no compatible model appears in both AI Flight Controller registry and Olla model discovery")
}

func isEmbeddingRoute(name, framework string) bool {
	lower := strings.ToLower(name + " " + framework)
	return strings.Contains(lower, "embed") || strings.Contains(lower, "bge") || strings.Contains(lower, "e5")
}

func isFleetModelReady(model fleetModel) bool {
	return statusReady(model.Status) && statusReady(model.State)
}

func statusReady(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "ready", "running", "healthy", "available", "active", "started":
		return true
	default:
		return false
	}
}

func requiresChatReadiness(purpose string) bool {
	switch purpose {
	case "", "generation", "scoring":
		return true
	default:
		return false
	}
}

func validateOllaChatRoute(ctx context.Context, client *http.Client, baseURL, model string) error {
	body := map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": "LongMemEval route readiness probe. Reply with READY."}},
		"max_tokens":  8,
		"temperature": 0,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if len(payload.Choices) == 0 {
		return errors.New("readiness probe returned no choices")
	}
	if payload.Choices[0].Message.Content == "" && payload.Choices[0].Text == "" {
		return errors.New("readiness probe returned an empty choice")
	}
	return nil
}
