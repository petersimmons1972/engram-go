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
	GpuDriver  string   `json:"gpuDriver,omitempty"`  // nvidia (default) or rocm
	RenderDevice string `json:"renderDevice,omitempty"` // e.g. /dev/dri/renderD128 (rocm only)
}

// ModelHash returns an 8-char hash of just this model's spec.
// Used as the container's policy-version label so only changed
// models restart — not all models when any single spec changes.
func ModelHash(m ModelSpec) string {
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)[:8]
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
	Remove(ctx context.Context, modelName string) error
	Inspect(ctx context.Context, name string) (*ContainerInspect, error)
}
