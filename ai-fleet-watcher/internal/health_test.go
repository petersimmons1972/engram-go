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
	if !strings.Contains(test[1], `"object":"embedding"`) {
		t.Errorf("infinity probe must grep for object:embedding (Infinity response format), got: %s", test[1])
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

func TestHealthConfigInfinityWithURLPrefix(t *testing.T) {
	spec := ModelSpec{
		Framework: "infinity",
		Port:      8004,
		Repo:      "BAAI/bge-m3",
		ExtraArgs: []string{"--url-prefix", "/v1", "--host", "0.0.0.0"},
	}
	hc := healthConfig(spec)
	if hc == nil {
		t.Fatal("expected non-nil healthConfig")
	}
	test := hc["Test"].([]string)
	if !strings.Contains(test[1], "/v1/embeddings") {
		t.Errorf("probe must use /v1/embeddings when --url-prefix /v1 set, got: %s", test[1])
	}
}

func TestHealthConfigInfinityNoPrefix(t *testing.T) {
	spec := ModelSpec{Framework: "infinity", Port: 8001, Repo: "BAAI/bge-m3"}
	hc := healthConfig(spec)
	test := hc["Test"].([]string)
	if !strings.Contains(test[1], "localhost:8001/embeddings") {
		t.Errorf("probe without prefix must use /embeddings directly, got: %s", test[1])
	}
	if strings.Contains(test[1], "//embeddings") {
		t.Errorf("double slash in probe URL: %s", test[1])
	}
}

func TestModelSpecStopTimeoutDefault(t *testing.T) {
	spec := ModelSpec{Framework: "infinity", GpuDriver: "rocm"}
	if spec.StopTimeoutSec != 0 {
		t.Errorf("zero-value StopTimeoutSec must be 0, got %d", spec.StopTimeoutSec)
	}
}

func TestModelSpecStopTimeoutHelper(t *testing.T) {
	cases := []struct {
		sec  int
		want int
	}{
		{0, 10},  // zero → Docker default (10s)
		{30, 30}, // explicit override
		{60, 60}, // larger override
	}
	for _, c := range cases {
		spec := ModelSpec{StopTimeoutSec: c.sec}
		if got := spec.stopTimeout(); got != c.want {
			t.Errorf("stopTimeout() with StopTimeoutSec=%d: got %d, want %d", c.sec, got, c.want)
		}
	}
}

func TestInfinityURLPrefix(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--url-prefix", "/v1"}, "/v1"},
		{[]string{"--host", "0.0.0.0", "--url-prefix", "/api"}, "/api"},
		{[]string{"--host", "0.0.0.0"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		spec := ModelSpec{ExtraArgs: c.args}
		got := infinityURLPrefix(spec)
		if got != c.want {
			t.Errorf("infinityURLPrefix(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}
