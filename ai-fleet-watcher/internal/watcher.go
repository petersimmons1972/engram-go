package internal

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const (
	restartWindow       = 10 * time.Minute
	maxRestartsInWindow = 3
)

type Watcher struct {
	hostname       string
	controllerURL  string
	docker         DockerClient
	currentVersion string
	pollInterval   time.Duration
	reportInterval time.Duration

	// Health-driven recovery state. All fields are single-goroutine — only
	// accessed from the Run() select loop. No locking needed.
	restartRecords map[string]restartRecord // keyed by container name
	degraded       map[string]time.Time     // keyed by container name; set when restart limit hit
	specsByName    map[string]ModelSpec     // keyed by model name (without "ai-fleet-" prefix)
}

func NewWatcher(hostname, controllerURL string, docker DockerClient) *Watcher {
	return &Watcher{
		hostname:       hostname,
		controllerURL:  controllerURL,
		docker:         docker,
		pollInterval:   60 * time.Second,
		reportInterval: 30 * time.Second,
		restartRecords: make(map[string]restartRecord),
		degraded:       make(map[string]time.Time),
		specsByName:    make(map[string]ModelSpec),
	}
}

func (w *Watcher) Run(ctx context.Context) {
	// Boot: apply last-known policy immediately so containers start
	// even if the controller is temporarily unreachable.
	if last := LoadLastPolicy(); last != nil {
		slog.Info("applying last-known policy on boot", "version", last.PolicyVersion)
		w.applyPolicy(ctx, last)
		w.currentVersion = last.PolicyVersion
	}

	// Attempt an immediate fetch to get current policy before first tick.
	w.poll(ctx)

	pollTick := time.NewTicker(w.pollInterval)
	reportTick := time.NewTicker(w.reportInterval)
	defer pollTick.Stop()
	defer reportTick.Stop()

	for {
		select {
		case <-pollTick.C:
			w.poll(ctx)
		case <-reportTick.C:
			w.checkHealth(ctx)
			w.report(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) poll(ctx context.Context) {
	policy, err := FetchPolicy(ctx, w.controllerURL, w.hostname)
	if err != nil {
		slog.Warn("fetch policy failed — keeping current", "err", err)
		return
	}
	if policy == nil {
		slog.Warn("no policy for this host yet", "hostname", w.hostname)
		return
	}
	if policy.PolicyVersion == w.currentVersion {
		return // nothing changed
	}
	slog.Info("policy changed", "old", w.currentVersion, "new", policy.PolicyVersion)
	w.applyPolicy(ctx, policy)
	SavePolicy(policy)
	w.currentVersion = policy.PolicyVersion
}

func (w *Watcher) applyPolicy(ctx context.Context, policy *Policy) {
	// Ensure NFS volume exists before starting any containers.
	if err := w.docker.EnsureNFSVolume(ctx, policy.Spec.NFSMount); err != nil {
		slog.Error("ensure NFS volume", "err", err)
		return
	}

	// Build set of desired model names and update specsByName for checkHealth().
	desired := make(map[string]ModelSpec, len(policy.Spec.Models))
	w.specsByName = make(map[string]ModelSpec, len(policy.Spec.Models))
	for _, m := range policy.Spec.Models {
		desired[m.Name] = m
		w.specsByName[m.Name] = m
	}

	// Remove containers not in policy.
	running, err := w.docker.ListManaged(ctx)
	if err != nil {
		slog.Error("list managed containers", "err", err)
	}
	for _, c := range running {
		modelName := c.Name[len("ai-fleet-"):] // strip prefix
		if _, ok := desired[modelName]; !ok {
			slog.Info("removing container not in policy", "name", c.Name)
			if err := w.docker.Remove(ctx, modelName); err != nil {
				slog.Error("remove container", "name", c.Name, "err", err)
			}
		}
	}

	// Start/update each desired container.
	for _, spec := range policy.Spec.Models {
		if err := w.docker.Ensure(ctx, spec, ModelHash(spec)); err != nil {
			slog.Error("ensure container", "model", spec.Name, "err", err)
		}
	}
}

// checkHealth inspects every managed container for Docker HEALTHCHECK status.
// Unhealthy containers are restarted via Remove+Ensure up to maxRestartsInWindow
// times within restartWindow. After the limit, the container is marked degraded
// and no further auto-restarts are attempted.
func (w *Watcher) checkHealth(ctx context.Context) {
	containers, err := w.docker.ListManaged(ctx)
	if err != nil {
		slog.Warn("checkHealth: list managed", "err", err)
		return
	}
	for _, c := range containers {
		info, err := w.docker.Inspect(ctx, c.Name)
		if err != nil {
			slog.Warn("checkHealth: inspect failed", "name", c.Name, "err", err)
			continue
		}
		if info == nil || info.State.Health == nil {
			continue // no healthcheck configured — skip
		}
		if info.State.Health.Status != "unhealthy" {
			delete(w.restartRecords, c.Name) // clear on recovery
			continue
		}

		rec := w.restartRecords[c.Name]
		if time.Since(rec.windowStart) > restartWindow {
			rec = restartRecord{windowStart: time.Now()} // reset sliding window
		}
		if rec.count >= maxRestartsInWindow {
			slog.Error("container restart limit reached — marking degraded, stopping auto-restart",
				"name", c.Name, "restarts", rec.count, "window", restartWindow)
			w.degraded[c.Name] = time.Now()
			continue
		}

		slog.Warn("container unhealthy — restarting",
			"name", c.Name, "attempt", rec.count+1, "of", maxRestartsInWindow)

		modelName := strings.TrimPrefix(c.Name, "ai-fleet-")
		spec, ok := w.specsByName[modelName]
		if !ok {
			slog.Warn("checkHealth: no spec for container — cannot restart", "name", c.Name)
			continue
		}
		if err := w.docker.Remove(ctx, modelName); err != nil {
			slog.Error("checkHealth: remove failed", "name", c.Name, "err", err)
		}
		if err := w.docker.Ensure(ctx, spec, ModelHash(spec)); err != nil {
			slog.Error("checkHealth: ensure failed", "name", c.Name, "err", err)
		}
		rec.count++
		w.restartRecords[c.Name] = rec
	}
}

func (w *Watcher) report(ctx context.Context) {
	containers, err := w.docker.ListManaged(ctx)
	if err != nil {
		slog.Warn("list managed for report", "err", err)
		return
	}
	degraded := make([]string, 0, len(w.degraded))
	for name := range w.degraded {
		degraded = append(degraded, name)
	}
	PostStatus(ctx, w.controllerURL, StatusReport{
		Hostname:           w.hostname,
		PolicyVersion:      w.currentVersion,
		Containers:         containers,
		DegradedContainers: degraded,
	})
}
