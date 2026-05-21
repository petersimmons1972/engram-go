package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type ModelSpec struct {
	Name      string   `json:"name"`
	Framework string   `json:"framework"`
	Repo      string   `json:"repo"`
	Dtype     string   `json:"dtype,omitempty"`
	Task      string   `json:"task,omitempty"`
	Port      int      `json:"port"`
	Image     string   `json:"image"`
	ExtraArgs  []string `json:"extraArgs,omitempty"`
	// EnvVars holds additional KEY=VALUE pairs injected into the container environment.
	// Use for secrets (e.g. VLLM_API_KEY) that should not appear in CLI args or
	// container labels. Values are sourced from the GPUHost CRD spec field envVars.
	EnvVars      []string `json:"envVars,omitempty"`
	GpuDriver  string   `json:"gpuDriver,omitempty"`  // nvidia (default) or rocm
	RenderDevice string `json:"renderDevice,omitempty"` // e.g. /dev/dri/renderD128 (rocm only)
	// ReadinessTimeoutSec is how long to wait for the model to become ready
	// before marking the container degraded. Used as Docker HEALTHCHECK StartPeriod.
	// 0 = fall back to 900s default (sufficient for Qwen3-32B NFS cold load).
	ReadinessTimeoutSec int `json:"readinessTimeoutSec,omitempty"`
	// StopTimeoutSec is seconds to wait for graceful SIGTERM before SIGKILL.
	// 0 = use Docker default (10s). Set to 30 for ROCm containers so HIP
	// worker threads can release /dev/kfd before SIGKILL.
	StopTimeoutSec int `json:"stopTimeoutSec,omitempty"`
}

// ModelHash returns an 8-char hash of just this model's spec.
// Used as the container's policy-version label so only changed
// models restart — not all models when any single spec changes.
func ModelHash(m ModelSpec) string {
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)[:8]
}

// stopTimeout returns the effective stop timeout in seconds.
// 0 on the spec means use Docker's default (10s).
func (m ModelSpec) stopTimeout() int {
	if m.StopTimeoutSec > 0 {
		return m.StopTimeoutSec
	}
	return 10 // Docker default
}

type GPUHostSpec struct {
	Host             string      `json:"host"`
	ReservedMemoryGB int         `json:"reservedMemoryGB"`
	NFSMount         string      `json:"nfsMount"`
	Models           []ModelSpec `json:"models"`
}

type Policy struct {
	Hostname      string      `json:"hostname"`
	Spec          GPUHostSpec `json:"spec"`
	PolicyVersion string      `json:"policyVersion"`
}

type ContainerStatus struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Port    int    `json:"port"`
	Running bool   `json:"running"`
}

type StatusReport struct {
	Hostname           string            `json:"hostname"`
	PolicyVersion      string            `json:"policyVersion"`
	Containers         []ContainerStatus `json:"containers"`
	DegradedContainers []string          `json:"degradedContainers,omitempty"`
	ReportedAt         time.Time         `json:"reportedAt"`
}

const labelManaged = "ai.petersimmons.com/managed"
const labelPolicyVersion = "ai.petersimmons.com/policy-version"
const nfsVolumeName = "ai-fleet-models"

func ContainerName(modelName string) string {
	return "ai-fleet-" + modelName
}

// ContainerInspect holds the fields we care about from GET /containers/{name}/json.
// Health is nil when no HEALTHCHECK is configured on the container.
type ContainerInspect struct {
	State struct {
		Running bool
		Health  *struct {
			Status string // "starting" | "healthy" | "unhealthy"
		}
	}
}

// restartRecord tracks unhealthy-restart history within a sliding window.
// Keyed by container name in Watcher.restartRecords.
type restartRecord struct {
	count       int
	windowStart time.Time
}

// DockerClient is the interface Watcher uses to manage containers.
// DockerManager implements it. Tests use fakes.
type DockerClient interface {
	EnsureNFSVolume(ctx context.Context, nfsMount string) error
	ListManaged(ctx context.Context) ([]ContainerStatus, error)
	Ensure(ctx context.Context, spec ModelSpec, policyVersion string) error
	// Remove stops the container gracefully (SIGTERM for stopTimeoutSec seconds)
	// then force-removes it. stopTimeoutSec=0 uses Docker default (10s).
	Remove(ctx context.Context, modelName string, stopTimeoutSec int) error
	Inspect(ctx context.Context, name string) (*ContainerInspect, error)
}
