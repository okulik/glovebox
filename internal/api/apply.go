package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

// projectMu serializes concurrent applies for the same project id.
var projectMu sync.Map // pid → *sync.Mutex

func projectLock(pid string) (*sync.Mutex, error) {
	v, _ := projectMu.LoadOrStore(pid, &sync.Mutex{})
	m, ok := v.(*sync.Mutex)
	if !ok {
		return nil, fmt.Errorf("map for pid '%s' contains no mutex", pid)
	}
	return m, nil
}

// forgetProjectLock removes the mutex entry for a project that has been
// destroyed. The caller must hold the mutex (because we're about to discard
// it). Without this, projectMu grows unbounded over the controller's lifetime
// across create/destroy churn.
func forgetProjectLock(pid string) {
	projectMu.Delete(pid)
}

// createAndStart creates a container per spec on netName and starts it.
// Returns the container ID, the phase ("create" or "start") at which it
// failed, and the error. On success returns (id, "", nil).
//
// When create fails the returned id is "". When start fails the returned id
// is the created container's ID, so callers can register it for cleanup
// before unwinding.
//
// Shared between apply (where the surrounding rollback uses "container_…_failed"
// error codes) and reset (where the surrounding error codes are "recreate_failed"
// / "restart_failed") - each caller maps `phase` to its own error code.
func createAndStart(ctx context.Context, dk dockerx.ControllerClient, spec dockerx.ContainerSpec, netName string) (id, phase string, err error) {
	id, err = dk.CreateContainer(ctx, spec, netName)
	if err != nil {
		return "", "create", err
	}
	if err := dk.StartContainer(ctx, id); err != nil {
		return id, "start", err
	}
	return id, "", nil
}

type applyDeps struct {
	docker         dockerx.ControllerClient
	state          *state.Store
	rules          manifest.Rules
	healthTimeout  time.Duration
	healthInterval time.Duration
	hostOnly       bool
}

// Deps is the exported mirror of applyDeps so main.go can build one.
type Deps struct {
	Docker dockerx.ControllerClient
	Store  *state.Store
	Rules  manifest.Rules
}

// pathVar reads a path variable from either r.PathValue (mux-routed)
// or a test override stored in the context. Tests use withPathVar to
// inject values without going through the mux.
func pathVar(r *http.Request, key string) string {
	if v, ok := r.Context().Value(pathVarKey(key)).(string); ok {
		return v
	}
	return r.PathValue(key)
}

// pathVarKey is the context-value key tests use to inject path variables
// without going through the mux. A named string type (rather than the bare
// string) prevents collisions with other packages' context keys.
type pathVarKey string

type applyHandler struct{ deps applyDeps }

func newApplyHandler(deps applyDeps) http.Handler { return &applyHandler{deps} }

func (h *applyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.deps.hostOnly {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	pid := pathVar(r, "pid")
	if pid == "" {
		writeError(w, http.StatusBadRequest, "missing_project_id", "")
		return
	}
	mu, err := projectLock(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_lock", err.Error())
		return
	}
	mu.Lock()
	defer mu.Unlock()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_body", err.Error())
		return
	}

	var m *manifest.Manifest
	var appliedYAML string
	if len(body) == 0 {
		// Empty body ⇒ apply the stored proposal (already validated at propose
		// time). This is how `gbx stack apply` promotes a proposal to live.
		if h.deps.state == nil {
			writeError(w, http.StatusBadRequest, "no_proposal", "no stored proposal to apply")
			return
		}
		rec, ok := h.deps.state.Get(pid)
		if !ok || rec.Proposed == nil {
			writeError(w, http.StatusBadRequest, "no_proposal", "no stored proposal to apply")
			return
		}
		m = rec.Proposed
		appliedYAML = rec.ProposedYAML
	} else {
		parsed, perr := manifest.Parse(body, h.deps.rules)
		if verr := manifest.AsValidationError(perr); verr != nil {
			writeValidationError(w, verr)
			return
		}
		if perr != nil {
			writeError(w, http.StatusBadRequest, "parse_failed", perr.Error())
			return
		}
		m = parsed
		appliedYAML = string(body)
	}

	plan := dockerx.Plan(pid, m)
	ctx := r.Context()

	type undo struct {
		volumes        []string
		containers     []string
		networkCreated bool
	}
	u := undo{}
	rollback := func() {
		for _, n := range u.containers {
			id, _, _ := h.deps.docker.ContainerByName(ctx, n)
			if id != "" {
				_ = h.deps.docker.StopContainer(ctx, id)
				_ = h.deps.docker.RemoveContainer(ctx, id, true)
			}
		}
		for _, v := range u.volumes {
			_ = h.deps.docker.RemoveVolume(ctx, v)
		}
		if u.networkCreated {
			_ = h.deps.docker.RemoveNetwork(ctx, plan.NetworkName)
		}
	}

	// 1. Network.
	if err := h.deps.docker.EnsureNetwork(ctx, plan.NetworkName, true); err != nil {
		writeError(w, http.StatusInternalServerError, "network_create_failed", err.Error())
		return
	}
	u.networkCreated = true

	// 2. Volumes.
	for _, v := range plan.Volumes {
		if err := h.deps.docker.EnsureVolume(ctx, v); err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, "volume_create_failed", err.Error())
			return
		}
		u.volumes = append(u.volumes, v)
	}

	// 3. Determine which containers need creating (idempotent re-apply).
	toCreate := make([]dockerx.ContainerSpec, 0, len(plan.Containers))
	for _, c := range plan.Containers {
		existingID, _, _ := h.deps.docker.ContainerByName(ctx, c.Name)
		if existingID == "" {
			toCreate = append(toCreate, c)
		}
	}

	// 4. Image pulls for new containers only.
	for _, c := range toCreate {
		if err := h.deps.docker.PullImage(ctx, c.Image); err != nil {
			rollback()
			writeError(w, http.StatusBadGateway, "image_pull_failed", err.Error())
			return
		}
	}

	// 5. Container create + start for new containers only.
	for _, c := range toCreate {
		id, phase, err := createAndStart(ctx, h.deps.docker, c, plan.NetworkName)
		if id != "" {
			// Created (regardless of whether Start succeeded) → track for cleanup.
			u.containers = append(u.containers, c.Name)
		}
		if err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, "container_"+phase+"_failed", err.Error())
			return
		}
	}

	// 6. Wait for healthchecks. context.WithTimeout bounds the whole wait
	// (across all services) and a ticker drives the poll cadence; both are
	// interrupted cleanly if the client disconnects or the controller shuts
	// down, unlike a time.Sleep loop.
	timeout := h.deps.healthTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	interval := h.deps.healthInterval
	if interval == 0 {
		interval = time.Second
	}
	hcCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for _, c := range plan.Containers {
		id, _, _ := h.deps.docker.ContainerByName(hcCtx, c.Name)
		var lastState string
		for {
			st, err := h.deps.docker.HealthState(hcCtx, id)
			if err != nil {
				rollback()
				writeError(w, http.StatusInternalServerError, "health_check_failed", err.Error())
				return
			}
			// "" means no healthcheck; treat running as healthy.
			if st == "" || st == "healthy" {
				break
			}
			lastState = st
			select {
			case <-hcCtx.Done():
				rollback()
				writeError(w, http.StatusBadGateway, "service_unhealthy",
					"service "+c.Name+" did not become healthy in time (last: "+lastState+")")
				return
			case <-ticker.C:
			}
		}
	}

	// 7. Attach the per-project agent container to the stack network so it can
	// resolve service DNS names. Done after healthchecks (rather than before
	// service creation) so a failed apply leaves no orphan endpoint blocking
	// the rollback's RemoveNetwork. If the agent container isn't present at
	// apply time, skip silently - reconcile-on-startup or the next apply will
	// pick it up once it exists.
	agentName := "glovebox-agent-" + pid
	if id, _, _ := h.deps.docker.ContainerByName(ctx, agentName); id != "" {
		if err := h.deps.docker.ConnectNetwork(ctx, agentName, plan.NetworkName); err != nil {
			rollback()
			writeError(w, http.StatusInternalServerError, "agent_attach_failed", err.Error())
			return
		}
	}

	if h.deps.state != nil {
		_ = h.deps.state.SaveApplied(pid, m, appliedYAML, "applied", "")
		// A successful apply consumes any pending proposal: live now reflects
		// what was applied, so the proposal is no longer meaningful.
		_ = h.deps.state.ClearProposed(pid)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "applied",
		"project_id": pid,
		"network":    plan.NetworkName,
		"services":   len(plan.Containers),
	})
}
