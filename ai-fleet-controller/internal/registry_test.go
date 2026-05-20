package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistryExposesDegradedContainers(t *testing.T) {
	store := NewStore()
	store.Set(StatusReport{
		Hostname:           "leviathan",
		PolicyVersion:      "v1",
		Containers:         []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		DegradedContainers: []string{"ai-fleet-embed"},
		ReportedAt:         time.Now(),
	})

	entry := RegistryEntry{Host: "leviathan", PolicyVersion: "v1"}
	if report, ok := store.Get("leviathan"); ok {
		entry.LastSeen = report.ReportedAt
		entry.Containers = report.Containers
		entry.DegradedContainers = report.DegradedContainers
	}

	b, _ := json.Marshal(entry)
	var out map[string]any
	json.Unmarshal(b, &out)

	degraded, ok := out["degradedContainers"]
	if !ok {
		t.Fatal("degradedContainers key missing from RegistryEntry JSON")
	}
	arr, ok := degraded.([]any)
	if !ok || len(arr) != 1 || arr[0] != "ai-fleet-embed" {
		t.Fatalf("unexpected degradedContainers value: %v", degraded)
	}
}

func TestHealthEndpointReturns200(t *testing.T) {
	srv := NewServer(nil, NewStore())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health endpoint returned %d", w.Code)
	}
}
