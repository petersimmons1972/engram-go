package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverRouteSelectsModelPresentInFleetAndOlla(t *testing.T) {
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/registry" {
			t.Fatalf("fleet path = %s, want /registry", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]fleetHost{
			{Host: "oblivion", Models: []fleetModel{{Name: "inference", Framework: "vllm", Port: 8000}}},
			{Host: "precision", Models: []fleetModel{{Name: "fast-inference", Framework: "vllm", Port: 8008}}},
		})
	}))
	defer fleet.Close()
	olla := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/olla/openai/v1/models" {
			t.Fatalf("olla path = %s, want /olla/openai/v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(openAIModelsResponse{
			Object: "list",
			Data: []openAIModel{
				{ID: "not-in-fleet"},
				{ID: "fast-inference"},
				{ID: "inference"},
			},
		})
	}))
	defer olla.Close()

	result, err := discoverRoute(routeDiscoverConfig{
		FleetURL: fleet.URL,
		OllaURL:  olla.URL,
		Purpose:  "generation",
	})
	if err != nil {
		t.Fatalf("discoverRoute: %v", err)
	}
	if result.LLMBaseURL != olla.URL+"/olla/openai/v1" {
		t.Fatalf("LLMBaseURL = %q", result.LLMBaseURL)
	}
	if result.LLMModel != "fast-inference" {
		t.Fatalf("LLMModel = %q, want first Olla model also present in fleet", result.LLMModel)
	}
	if len(result.FleetHosts) != 2 || len(result.OllaModels) != 3 {
		t.Fatalf("unexpected result shape: %+v", result)
	}
}

func TestDiscoverRouteRejectsRequestedModelAbsentFromOlla(t *testing.T) {
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]fleetHost{
			{Host: "oblivion", Models: []fleetModel{{Name: "inference", Framework: "vllm", Port: 8000}}},
		})
	}))
	defer fleet.Close()
	olla := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openAIModelsResponse{
			Object: "list",
			Data:   []openAIModel{{ID: "inference"}},
		})
	}))
	defer olla.Close()

	_, err := discoverRoute(routeDiscoverConfig{
		FleetURL: fleet.URL,
		OllaURL:  olla.URL,
		Model:    "missing",
		Purpose:  "generation",
	})
	if err == nil {
		t.Fatal("discoverRoute returned nil error for model absent from Olla")
	}
}

func TestDiscoverRouteAvoidsEmbeddingModelsForGeneration(t *testing.T) {
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]fleetHost{
			{Host: "precision", Models: []fleetModel{
				{Name: "BAAI/bge-m3", Framework: "llama-cpp", Port: 8005},
				{Name: "inference", Framework: "vllm", Port: 8008},
			}},
		})
	}))
	defer fleet.Close()
	olla := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openAIModelsResponse{
			Object: "list",
			Data:   []openAIModel{{ID: "BAAI/bge-m3"}, {ID: "inference"}},
		})
	}))
	defer olla.Close()

	result, err := discoverRoute(routeDiscoverConfig{
		FleetURL: fleet.URL,
		OllaURL:  olla.URL,
		Purpose:  "generation",
	})
	if err != nil {
		t.Fatalf("discoverRoute: %v", err)
	}
	if result.LLMModel != "inference" {
		t.Fatalf("LLMModel = %q, want generation model", result.LLMModel)
	}
}
