package api

import (
	"net/http"
	"strconv"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
)

// liveManifest returns the manifest currently associated with pid in the
// store, or nil if there is none.
func liveManifest(deps applyDeps, pid string) *manifest.Manifest {
	if deps.state == nil {
		return nil
	}
	rec, ok := deps.state.Get(pid)
	if !ok {
		return nil
	}
	return rec.Manifest
}

type serviceHandler struct {
	op   string
	deps applyDeps
}

func newServiceHandler(deps applyDeps, op string) http.Handler {
	return &serviceHandler{op, deps}
}

func (h *serviceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	svc := pathVar(r, "svc")
	m := liveManifest(h.deps, pid)
	if m == nil {
		writeError(w, http.StatusNotFound, "no_live_manifest", "no live manifest for project")
		return
	}
	if _, ok := m.Services[svc]; !ok {
		writeAPIError(w, http.StatusNotFound, APIError{
			Code:         "service_not_in_manifest",
			Message:      svc + " is not declared in the live manifest.",
			Path:         "services." + svc,
			HintForAgent: "gbx stack propose <file> with " + svc + " added, then gbx stack wait.",
		})
		return
	}
	name := config.ContainerStackPrefix + pid + "-" + svc
	id, _, err := h.deps.docker.ContainerByName(r.Context(), name)
	if err != nil || id == "" {
		writeError(w, http.StatusNotFound, "container_missing", name)
		return
	}
	switch h.op {
	case "start":
		if err := h.deps.docker.StartContainer(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "start_failed", err.Error())
			return
		}
	case "stop":
		if err := h.deps.docker.StopContainer(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": h.op})
}

type resetHandler struct{ deps applyDeps }

func newResetHandler(deps applyDeps) http.Handler { return &resetHandler{deps} }

func (h *resetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	svc := pathVar(r, "svc")
	m := liveManifest(h.deps, pid)
	if m == nil {
		writeError(w, http.StatusNotFound, "no_live_manifest", "")
		return
	}
	cfg, ok := m.Services[svc]
	if !ok {
		writeError(w, http.StatusNotFound, "service_not_in_manifest", svc)
		return
	}

	mu, err := projectLock(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_lock", err.Error())
		return
	}
	mu.Lock()
	defer mu.Unlock()

	ctx := r.Context()
	name := config.ContainerStackPrefix + pid + "-" + svc
	plan := dockerx.Plan(pid, &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{svc: cfg}})

	// Reset is inherently destructive (the point is to wipe data), so it is
	// not fully transactional in the apply sense - a wiped volume cannot be
	// un-wiped. What is enforced here:
	//
	//   * Step 1 (stop+remove old container) errors block the rest, so we
	//     never have two containers fighting over the same volumes.
	//   * Steps 2 and 3 errors call ensureVolumes() so volumes always exist
	//     after the operation - even after a failed recreate, an operator
	//     `gbx stack apply` can put the container back without losing more.
	//   * On any failure the response body names the phase that broke, so
	//     `gbx-stack wait` can show the agent a useful reason.
	ensureVolumes := func() {
		for _, v := range plan.Volumes {
			_ = h.deps.docker.EnsureVolume(ctx, v)
		}
	}

	// Phase 1: stop + remove the existing container.
	if id, _, _ := h.deps.docker.ContainerByName(ctx, name); id != "" {
		if err := h.deps.docker.StopContainer(ctx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
			return
		}
		if err := h.deps.docker.RemoveContainer(ctx, id, true); err != nil {
			writeError(w, http.StatusInternalServerError, "remove_failed", err.Error())
			return
		}
	}

	// Phase 2: wipe + re-create volumes.
	for _, v := range plan.Volumes {
		_ = h.deps.docker.RemoveVolume(ctx, v)
		if err := h.deps.docker.EnsureVolume(ctx, v); err != nil {
			ensureVolumes()
			writeError(w, http.StatusInternalServerError, "volume_recreate_failed", err.Error())
			return
		}
	}

	// Phase 3: create + start the new container.
	resetCodes := map[string]string{"create": "recreate_failed", "start": "restart_failed"}
	for _, c := range plan.Containers {
		if _, phase, err := createAndStart(ctx, h.deps.docker, c, plan.NetworkName); err != nil {
			ensureVolumes()
			writeError(w, http.StatusInternalServerError, resetCodes[phase], err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

type infoHandler struct{ deps applyDeps }

func newInfoHandler(deps applyDeps) http.Handler { return &infoHandler{deps} }

func (h *infoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	m := liveManifest(h.deps, pid)
	if m == nil {
		writeError(w, http.StatusNotFound, "no_live_manifest", "")
		return
	}
	out := map[string]map[string]any{}
	for name, svc := range m.Services {
		out[name] = map[string]any{
			"host":  name,
			"port":  dockerx.DefaultPortFor(svc.Image),
			"image": svc.Image,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"project_id": pid, "services": out})
}

type logsHandler struct{ deps applyDeps }

func newLogsHandler(deps applyDeps) http.Handler { return &logsHandler{deps} }

func (h *logsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	svc := pathVar(r, "svc")
	m := liveManifest(h.deps, pid)
	if m == nil {
		writeError(w, http.StatusNotFound, "no_live_manifest", "")
		return
	}
	if _, ok := m.Services[svc]; !ok {
		writeError(w, http.StatusNotFound, "service_not_in_manifest", svc)
		return
	}
	id, _, _ := h.deps.docker.ContainerByName(r.Context(), config.ContainerStackPrefix+pid+"-"+svc)
	if id == "" {
		writeError(w, http.StatusNotFound, "container_missing", "")
		return
	}
	q := r.URL.Query()
	tail := 0
	if v := q.Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			tail = n
		}
	}
	follow := q.Get("follow") == "true"
	w.Header().Set("Content-Type", "text/plain")
	if err := h.deps.docker.Logs(r.Context(), id, tail, follow, w); err != nil {
		return
	}
}
