package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// engramProjects is the list of Engram projects scanned for instinct patterns.
// Matches instinct-python/cmd/audit/main.go:123. Do NOT add new projects without
// invoking advisory-gate (A1: changes the scan scope).
var engramProjects = []string{"psimmons", "global", "clearwatch", "homelab", "engram"}

// fetchPatterns queries each Engram project for instinct-tagged memories and
// returns the deduplicated set. Only records that have BOTH a "sig-*" tag AND
// an "instinct" tag are included.
//
// Ported verbatim from instinct-python/cmd/audit/main.go:119-161 with the
// only change being that the Ollama import is gone — this function is pure HTTP.
func fetchPatterns(base, token string) ([]engramMemory, error) {
	seen := map[string]bool{}
	var out []engramMemory

	for _, project := range engramProjects {
		body := map[string]any{
			"query":   "PROVENANCE observed instinct behaviour pattern first seen",
			"project": project,
			"limit":   50,
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, base+"/quick-recall", bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("quick-recall %s: build request: %w", project, err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("quick-recall %s: %w", project, err)
		}
		var payload struct {
			Results []engramMemory `json:"results"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()

		for _, m := range payload.Results {
			hasSig := false
			hasInstinct := false
			for _, t := range m.Tags {
				if strings.HasPrefix(t, "sig-") {
					hasSig = true
				}
				if t == "instinct" {
					hasInstinct = true
				}
			}
			if hasSig && hasInstinct && !seen[m.ID] {
				seen[m.ID] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}
