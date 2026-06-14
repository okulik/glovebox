package api

import (
	"net/http"
	"strings"
)

type statusHandler struct{ deps applyDeps }

func newStatusHandler(deps applyDeps) http.Handler { return &statusHandler{deps} }

func (h *statusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	netName := "glovebox-stack-" + pid
	prefix := netName + "-"

	containers, err := h.deps.docker.ListContainersByPrefix(r.Context(), prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	services := map[string]any{}
	state := "down"
	running := 0
	for _, c := range containers {
		svc := strings.TrimPrefix(c.Name, prefix)
		health, _ := h.deps.docker.HealthState(r.Context(), c.ID)
		services[svc] = map[string]string{"state": c.State, "health": health}
		if c.State == "running" {
			running++
		}
	}
	if len(containers) > 0 {
		if running == len(containers) {
			state = "ready"
		} else {
			state = "degraded"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"state":    state,
		"services": services,
		"network":  netName,
	})
}

type downHandler struct{ deps applyDeps }

func newDownHandler(deps applyDeps) http.Handler { return &downHandler{deps} }

func (h *downHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	mu, err := projectLock(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_lock", err.Error())
		return
	}
	mu.Lock()
	defer mu.Unlock()
	prefix := "glovebox-stack-" + pid + "-"
	containers, err := h.deps.docker.ListContainersByPrefix(r.Context(), prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	for _, c := range containers {
		_ = h.deps.docker.StopContainer(r.Context(), c.ID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "down"})
}

type destroyHandler struct{ deps applyDeps }

func newDestroyHandler(deps applyDeps) http.Handler { return &destroyHandler{deps} }

func (h *destroyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, http.StatusBadRequest, "confirm_required", "pass ?confirm=true to destroy")
		return
	}
	pid := pathVar(r, "pid")
	mu, err := projectLock(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_lock", err.Error())
		return
	}
	mu.Lock()
	defer mu.Unlock()
	ctx := r.Context()
	prefix := "glovebox-stack-" + pid + "-"
	containers, err := h.deps.docker.ListContainersByPrefix(ctx, prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	for _, c := range containers {
		_ = h.deps.docker.StopContainer(ctx, c.ID)
		_ = h.deps.docker.RemoveContainer(ctx, c.ID, true)
	}
	volumes, err := h.deps.docker.ListVolumesByPrefix(ctx, prefix)
	if err == nil {
		for _, v := range volumes {
			_ = h.deps.docker.RemoveVolume(ctx, v)
		}
	}
	_ = h.deps.docker.RemoveNetwork(ctx, "glovebox-stack-"+pid)
	if h.deps.state != nil {
		_ = h.deps.state.Delete(pid)
	}
	forgetProjectLock(pid)
	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

type listProjectsHandler struct{ deps applyDeps }

func newListProjectsHandler(deps applyDeps) http.Handler { return &listProjectsHandler{deps} }

func (h *listProjectsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if !h.deps.hostOnly {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	out := []map[string]any{}
	if h.deps.state != nil {
		for pid, rec := range h.deps.state.All() {
			row := map[string]any{
				"project_id": pid,
				"last_apply": rec.LastApply,
			}
			if rec.Manifest != nil {
				row["services"] = len(rec.Manifest.Services)
			} else {
				row["services"] = 0
			}
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}
