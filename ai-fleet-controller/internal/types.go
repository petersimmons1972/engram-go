package internal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type ModelSpec struct {
	Name      string   `json:"name"`
	Framework string   `json:"framework"` // vllm | infinity | llama-cpp
	Repo      string   `json:"repo"`
	Dtype     string   `json:"dtype,omitempty"`
	Task      string   `json:"task,omitempty"`
	Port      int      `json:"port"`
	Image     string   `json:"image"`
	ExtraArgs     []string `json:"extraArgs,omitempty"`
	// EnvVars holds KEY=VALUE pairs passed verbatim to the container environment.
	// Matches the CRD schema field envVars. Included in the policy hash so that
	// adding or changing env vars (e.g. VLLM_API_KEY) triggers a watcher reconcile.
	EnvVars       []string `json:"envVars,omitempty"`
	GpuDriver     string   `json:"gpuDriver,omitempty"`
	RenderDevice  string   `json:"renderDevice,omitempty"`
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

func SpecHash(spec GPUHostSpec) string {
	b, _ := json.Marshal(spec)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)[:8]
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

type RegistryEntry struct {
	Host               string            `json:"host"`
	PolicyVersion      string            `json:"policyVersion"`
	Models             []ModelSpec       `json:"models"`
	LastSeen           time.Time         `json:"lastSeen,omitempty"`
	Containers         []ContainerStatus `json:"containers,omitempty"`
	DegradedContainers []string          `json:"degradedContainers,omitempty"`
}
