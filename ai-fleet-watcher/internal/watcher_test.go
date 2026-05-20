package internal

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeDocker is a test double for DockerClient.
type fakeDocker struct {
	mu         sync.Mutex
	containers []ContainerStatus
	inspects   map[string]*ContainerInspect // keyed by container name
	removes    []string
	ensures    []string
}

func (f *fakeDocker) EnsureNFSVolume(_ context.Context, _ string) error { return nil }

func (f *fakeDocker) ListManaged(_ context.Context) ([]ContainerStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ContainerStatus(nil), f.containers...), nil
}

func (f *fakeDocker) Ensure(_ context.Context, spec ModelSpec, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensures = append(f.ensures, ContainerName(spec.Name))
	return nil
}

func (f *fakeDocker) Remove(_ context.Context, modelName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes = append(f.removes, ContainerName(modelName))
	return nil
}

func (f *fakeDocker) Inspect(_ context.Context, name string) (*ContainerInspect, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info, ok := f.inspects[name]
	if !ok {
		return nil, nil
	}
	return info, nil
}

func unhealthyInspect() *ContainerInspect {
	info := &ContainerInspect{}
	info.State.Running = true
	info.State.Health = &struct{ Status string }{"unhealthy"}
	return info
}

func healthyInspect() *ContainerInspect {
	info := &ContainerInspect{}
	info.State.Running = true
	info.State.Health = &struct{ Status string }{"healthy"}
	return info
}

func newTestWatcher(docker DockerClient) *Watcher {
	return &Watcher{
		hostname:       "test-host",
		controllerURL:  "http://localhost:0",
		docker:         docker,
		pollInterval:   time.Hour,
		reportInterval: time.Hour,
		restartRecords: make(map[string]restartRecord),
		degraded:       make(map[string]time.Time),
		specsByName: map[string]ModelSpec{
			"embed": {Name: "embed", Framework: "infinity", Port: 8001, Repo: "BAAI/bge-m3", Image: "michaelf34/infinity:latest-rocm"},
		},
	}
}

func TestCheckHealthRestartsUnhealthyContainer(t *testing.T) {
	fd := &fakeDocker{
		containers: []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		inspects:   map[string]*ContainerInspect{"ai-fleet-embed": unhealthyInspect()},
	}
	w := newTestWatcher(fd)
	w.checkHealth(context.Background())

	if len(fd.removes) != 1 || fd.removes[0] != "ai-fleet-embed" {
		t.Errorf("expected Remove(embed), got removes=%v", fd.removes)
	}
	if len(fd.ensures) != 1 || fd.ensures[0] != "ai-fleet-embed" {
		t.Errorf("expected Ensure(embed), got ensures=%v", fd.ensures)
	}
}

func TestCheckHealthBackoffStopsAtMax(t *testing.T) {
	fd := &fakeDocker{
		containers: []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		inspects:   map[string]*ContainerInspect{"ai-fleet-embed": unhealthyInspect()},
	}
	w := newTestWatcher(fd)
	w.restartRecords["ai-fleet-embed"] = restartRecord{
		count:       maxRestartsInWindow,
		windowStart: time.Now(),
	}
	w.checkHealth(context.Background())

	if len(fd.removes) != 0 {
		t.Errorf("should not restart when at limit, got removes=%v", fd.removes)
	}
	if _, degraded := w.degraded["ai-fleet-embed"]; !degraded {
		t.Error("container should be marked degraded after hitting limit")
	}
}

func TestCheckHealthWindowResetAllowsRestartAgain(t *testing.T) {
	fd := &fakeDocker{
		containers: []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		inspects:   map[string]*ContainerInspect{"ai-fleet-embed": unhealthyInspect()},
	}
	w := newTestWatcher(fd)
	w.restartRecords["ai-fleet-embed"] = restartRecord{
		count:       maxRestartsInWindow,
		windowStart: time.Now().Add(-(restartWindow + time.Second)),
	}
	w.checkHealth(context.Background())

	if len(fd.removes) != 1 {
		t.Errorf("window reset should allow restart, got removes=%v", fd.removes)
	}
}

func TestCheckHealthClearsRecordOnRecovery(t *testing.T) {
	fd := &fakeDocker{
		containers: []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		inspects:   map[string]*ContainerInspect{"ai-fleet-embed": healthyInspect()},
	}
	w := newTestWatcher(fd)
	w.restartRecords["ai-fleet-embed"] = restartRecord{count: 2, windowStart: time.Now()}

	w.checkHealth(context.Background())

	if _, exists := w.restartRecords["ai-fleet-embed"]; exists {
		t.Error("restart record should be cleared when container is healthy")
	}
	if len(fd.removes) != 0 {
		t.Error("should not restart a healthy container")
	}
}

func TestCheckHealthSkipsContainerWithNoHealthcheck(t *testing.T) {
	info := &ContainerInspect{}
	info.State.Running = true
	// info.State.Health is nil — no healthcheck configured

	fd := &fakeDocker{
		containers: []ContainerStatus{{Name: "ai-fleet-embed", Running: true}},
		inspects:   map[string]*ContainerInspect{"ai-fleet-embed": info},
	}
	w := newTestWatcher(fd)
	w.checkHealth(context.Background())

	if len(fd.removes) != 0 {
		t.Error("should not restart container with no healthcheck configured")
	}
}
