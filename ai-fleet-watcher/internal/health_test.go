package internal

import (
	"strings"
	"testing"
	"time"
)

func TestStatusReportHasDegradedContainersField(t *testing.T) {
	r := StatusReport{DegradedContainers: []string{"ai-fleet-embed"}}
	if len(r.DegradedContainers) != 1 || r.DegradedContainers[0] != "ai-fleet-embed" {
		t.Fatalf("DegradedContainers not set correctly: %v", r.DegradedContainers)
	}
}

func TestDockerManagerImplementsDockerClient(t *testing.T) {
	// Compile-time check: DockerManager must implement DockerClient.
	var _ DockerClient = (*DockerManager)(nil)
}

func TestHealthConfigInfinity(t *testing.T) {
	spec := ModelSpec{Framework: "infinity", Port: 8001, Repo: "BAAI/bge-m3"}
	hc := healthConfig(spec)
	if hc == nil {
		t.Fatal("expected non-nil healthConfig for infinity")
	}
	test, ok := hc["Test"].([]string)
	if !ok || len(test) != 2 || test[0] != "CMD-SHELL" {
		t.Fatalf("unexpected Test: %v", test)
	}
	if !strings.Contains(test[1], "POST") {
		t.Errorf("infinity probe must use POST /embeddings, got: %s", test[1])
	}
	if !strings.Contains(test[1], `"object":"list"`) {
		t.Errorf("infinity probe must grep for object:list, got: %s", test[1])
	}
	if hc["Retries"] != 2 {
		t.Errorf("infinity retries must be 2, got %v", hc["Retries"])
	}
	if hc["StartPeriod"] != int64(120*time.Second) {
		t.Errorf("infinity StartPeriod must be 120s nanoseconds, got %v", hc["StartPeriod"])
	}
}

func TestHealthConfigVLLM(t *testing.T) {
	spec := ModelSpec{Framework: "vllm", Port: 8000}
	hc := healthConfig(spec)
	if hc == nil {
		t.Fatal("expected non-nil healthConfig for vllm")
	}
	test := hc["Test"].([]string)
	if !strings.Contains(test[1], "/v1/models") {
		t.Errorf("vllm probe must hit /v1/models, got: %s", test[1])
	}
	if hc["Retries"] != 3 {
		t.Errorf("vllm retries must be 3, got %v", hc["Retries"])
	}
}

func TestHealthConfigLlamaCpp(t *testing.T) {
	spec := ModelSpec{Framework: "llama-cpp", Port: 8002}
	hc := healthConfig(spec)
	if hc == nil {
		t.Fatal("expected non-nil healthConfig for llama-cpp")
	}
	test := hc["Test"].([]string)
	if !strings.Contains(test[1], "/health") {
		t.Errorf("llama-cpp probe must hit /health, got: %s", test[1])
	}
}

func TestHealthConfigUnknownFramework(t *testing.T) {
	spec := ModelSpec{Framework: "unknown", Port: 9999}
	if healthConfig(spec) != nil {
		t.Fatal("unknown framework should return nil")
	}
}
