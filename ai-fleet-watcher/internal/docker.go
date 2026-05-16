package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DockerManager calls the Docker Engine HTTP API directly.
// DOCKER_HOST controls the endpoint: unix:///var/run/docker.sock (default)
// or tcp://host:port when using docker-socket-proxy.
type DockerManager struct {
	client  *http.Client
	baseURL string
}

func NewDockerManager() (*DockerManager, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	var transport http.RoundTripper
	var baseURL string

	if strings.HasPrefix(host, "unix://") {
		sockPath := strings.TrimPrefix(host, "unix://")
		transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
			},
		}
		baseURL = "http://localhost"
	} else {
		// tcp://host:port or http://host:port
		baseURL = strings.Replace(host, "tcp://", "http://", 1)
		transport = http.DefaultTransport
	}

	return &DockerManager{
		client:  &http.Client{Transport: transport, Timeout: 60 * time.Second},
		baseURL: baseURL,
	}, nil
}

func (d *DockerManager) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.baseURL+"/v1.47"+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return d.client.Do(req)
}

func (d *DockerManager) EnsureNFSVolume(ctx context.Context, nfsMount string) error {
	parts := strings.SplitN(nfsMount, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid nfsMount %q", nfsMount)
	}
	nfsHost, nfsPath := parts[0], parts[1]

	// Check if volume already exists.
	resp, err := d.do(ctx, http.MethodGet, "/volumes/"+nfsVolumeName, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Create it.
	payload := map[string]any{
		"Name":   nfsVolumeName,
		"Driver": "local",
		"DriverOpts": map[string]string{
			"type":   "nfs",
			"o":      fmt.Sprintf("addr=%s,rw,nfsvers=4", nfsHost),
			"device": ":" + nfsPath,
		},
	}
	resp, err = d.do(ctx, http.MethodPost, "/volumes/create", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create volume: %s", b)
	}
	slog.Info("created NFS volume", "name", nfsVolumeName, "host", nfsHost)
	return nil
}

func (d *DockerManager) ListManaged(ctx context.Context) ([]ContainerStatus, error) {
	resp, err := d.do(ctx, http.MethodGet,
		`/containers/json?all=true&filters={"label":["ai.petersimmons.com/managed=true"]}`, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var items []struct {
		Names  []string `json:"Names"`
		Image  string   `json:"Image"`
		State  string   `json:"State"`
		Ports  []struct {
			PrivatePort int `json:"PrivatePort"`
		} `json:"Ports"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	out := make([]ContainerStatus, 0, len(items))
	for _, c := range items {
		name := strings.TrimPrefix(c.Names[0], "/")
		port := 0
		if len(c.Ports) > 0 {
			port = c.Ports[0].PrivatePort
		}
		out = append(out, ContainerStatus{
			Name:    name,
			Image:   c.Image,
			Port:    port,
			Running: c.State == "running",
		})
	}
	return out, nil
}

func (d *DockerManager) Ensure(ctx context.Context, spec ModelSpec, policyVersion string) error {
	name := ContainerName(spec.Name)

	// Check if already up-to-date.
	resp, err := d.do(ctx, http.MethodGet, "/containers/"+name+"/json", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		var info struct {
			State  struct{ Running bool }
			Config struct {
				Labels map[string]string
			}
		}
		json.NewDecoder(resp.Body).Decode(&info)
		resp.Body.Close()
		if info.Config.Labels[labelPolicyVersion] == policyVersion && info.State.Running {
			slog.Info("container up-to-date", "name", name)
			return nil
		}
		slog.Info("removing stale container", "name", name)
		d.stopRemove(ctx, name)
	} else {
		resp.Body.Close()
	}

	// Pull image.
	slog.Info("pulling image", "image", spec.Image)
	img := strings.ReplaceAll(spec.Image, "/", "%2F")
	if idx := strings.Index(spec.Image, "/"); idx >= 0 {
		// Use fromImage parameter properly
		img = spec.Image
	}
	_ = img
	pullResp, err := d.do(ctx, http.MethodPost,
		"/images/create?fromImage="+urlEncode(spec.Image), nil)
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	io.Copy(io.Discard, pullResp.Body)
	pullResp.Body.Close()

	// Build create body.
	portStr := strconv.Itoa(spec.Port) + "/tcp"
	body := map[string]any{
		"Image": spec.Image,
		"Cmd":   modelArgs(spec),
		"Env": []string{
			"HF_HOME=/mnt/ai-models/hf-cache",
			"HUGGING_FACE_HUB_TOKEN=" + os.Getenv("HUGGING_FACE_HUB_TOKEN"),
		},
		"ExposedPorts": map[string]any{portStr: map[string]any{}},
		"Labels": map[string]string{
			labelManaged:       "true",
			labelPolicyVersion: policyVersion,
		},
		"HostConfig": map[string]any{
			"PortBindings": map[string]any{
				portStr: []map[string]string{{"HostIp": "0.0.0.0", "HostPort": strconv.Itoa(spec.Port)}},
			},
			"Mounts": []map[string]any{{
				"Type":   "volume",
				"Source": nfsVolumeName,
				"Target": "/mnt/ai-models",
			}},
			"RestartPolicy":  map[string]any{"Name": "unless-stopped"},
			"IpcMode":        "host",
			"DeviceRequests": gpuDeviceRequests(spec),
			"Devices":        rocmDeviceBinds(spec),
		},
	}

	if hc := healthConfig(spec); hc != nil {
		body["Healthcheck"] = hc
	}

	createResp, err := d.do(ctx, http.MethodPost, "/containers/create?name="+name, body)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("create container %s: %s", name, b)
	}

	var created struct{ Id string }
	json.NewDecoder(createResp.Body).Decode(&created)

	startResp, err := d.do(ctx, http.MethodPost, "/containers/"+created.Id+"/start", nil)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	startResp.Body.Close()
	slog.Info("started container", "name", name, "port", spec.Port)
	return nil
}

func (d *DockerManager) Remove(ctx context.Context, modelName string) error {
	name := ContainerName(modelName)
	d.stopRemove(ctx, name)
	return nil
}

// Inspect calls GET /containers/{name}/json and returns health status fields.
// Returns nil, nil when the container does not exist (404).
func (d *DockerManager) Inspect(ctx context.Context, name string) (*ContainerInspect, error) {
	resp, err := d.do(ctx, http.MethodGet, "/containers/"+name+"/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inspect container %s: %s", name, b)
	}
	var info ContainerInspect
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("inspect decode %s: %w", name, err)
	}
	return &info, nil
}

func (d *DockerManager) stopRemove(ctx context.Context, name string) {
	r, _ := d.do(ctx, http.MethodPost, "/containers/"+name+"/stop", nil)
	if r != nil { r.Body.Close() }
	r, _ = d.do(ctx, http.MethodDelete, "/containers/"+name+"?force=true", nil)
	if r != nil { r.Body.Close() }
}

func modelArgs(spec ModelSpec) []string {
	switch spec.Framework {
	case "vllm":
		args := []string{"--model", spec.Repo, "--host", "0.0.0.0", "--port", strconv.Itoa(spec.Port), "--served-model-name", spec.Name}
		if len(spec.ExtraArgs) > 0 {
			args = append(args, spec.ExtraArgs...)
		}
		if spec.Task != "" {
			args = append(args, "--task", spec.Task)
		}
		if spec.Dtype != "" {
			args = append(args, "--dtype", spec.Dtype)
		}
		return args
	case "infinity":
		args := []string{"v2", "--model-id", spec.Repo, "--port", strconv.Itoa(spec.Port)}
		if len(spec.ExtraArgs) > 0 {
			args = append(args, spec.ExtraArgs...)
		}
		return args
	case "llama-cpp":
		return []string{"--model", "/mnt/ai-models/hf-cache/" + spec.Repo, "--host", "0.0.0.0", "--port", strconv.Itoa(spec.Port), "-ngl", "99"}
	}
	return nil
}

func gpuDeviceRequests(spec ModelSpec) []map[string]any {
	if spec.Framework == "llama-cpp" || spec.GpuDriver == "rocm" {
		return nil
	}
	return []map[string]any{{
		"Driver":       "nvidia",
		"Count":        -1,
		"Capabilities": [][]string{{"gpu"}},
	}}
}

// rocmDeviceBinds returns /dev/kfd + /dev/dri/renderD* binds for ROCm GPUs.
// Only applied when GpuDriver == "rocm"; NVIDIA containers use DeviceRequests instead.
func rocmDeviceBinds(spec ModelSpec) []map[string]string {
	if spec.GpuDriver != "rocm" {
		return nil
	}
	renderDev := spec.RenderDevice
	if renderDev == "" {
		renderDev = "/dev/dri/renderD128"
	}
	bind := func(path string) map[string]string {
		return map[string]string{"PathOnHost": path, "PathInContainer": path, "CgroupPermissions": "rwm"}
	}
	return []map[string]string{bind("/dev/kfd"), bind(renderDev)}
}

// healthConfig returns the Docker Engine API HealthCheck body for the given framework.
// Returns nil for unknown frameworks — no healthcheck is configured.
//
// All durations are nanoseconds (Docker Engine API requirement; time.Duration is nanoseconds).
// Infinity uses POST /embeddings because GET /health does not exercise the GPU thread.
func healthConfig(spec ModelSpec) map[string]any {
	switch spec.Framework {
	case "infinity":
		return map[string]any{
			"Test": []string{"CMD-SHELL", fmt.Sprintf(
				`curl -sf -X POST http://localhost:%d/embeddings`+
					` -H 'Content-Type: application/json'`+
					` -d '{"model":"%s","input":["probe"]}'`+
					` --max-time 25 | grep -q '"object":"list"'`,
				spec.Port, spec.Repo,
			)},
			"Interval":    int64(60 * time.Second),
			"Timeout":     int64(30 * time.Second),
			"StartPeriod": int64(120 * time.Second), // bge-m3 loads in ~85-95s from NFS cache
			"Retries":     2,                         // 2 consecutive = GPU thread dead (filters transient 429s)
		}
	case "vllm":
		return map[string]any{
			"Test":        []string{"CMD-SHELL", fmt.Sprintf(`curl -sf http://localhost:%d/v1/models | grep -q '"id"'`, spec.Port)},
			"Interval":    int64(30 * time.Second),
			"Timeout":     int64(10 * time.Second),
			"StartPeriod": int64(180 * time.Second),
			"Retries":     3,
		}
	case "llama-cpp":
		return map[string]any{
			"Test":        []string{"CMD-SHELL", fmt.Sprintf(`curl -sf http://localhost:%d/health`, spec.Port)},
			"Interval":    int64(30 * time.Second),
			"Timeout":     int64(10 * time.Second),
			"StartPeriod": int64(60 * time.Second),
			"Retries":     3,
		}
	}
	return nil
}

func urlEncode(s string) string {
	return strings.NewReplacer("/", "%2F", ":", "%3A").Replace(s)
}
