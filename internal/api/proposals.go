package api

import (
	"io"
	"net/http"

	"github.com/okulik/glovebox/internal/manifest"
)

// proposeHandler stores a validated manifest proposal for a project. It is
// agent-callable (registered on registerCommon) so the in-container agent can
// suggest a stack, but the proposal has no effect until the host applies it.
type proposeHandler struct{ deps applyDeps }

func newProposeHandler(deps applyDeps) http.Handler { return &proposeHandler{deps} }

func (h *proposeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	if pid == "" {
		writeError(w, http.StatusBadRequest, "missing_project_id", "")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_body", err.Error())
		return
	}
	m, err := manifest.Parse(body, h.deps.rules)
	if verr := manifest.AsValidationError(err); verr != nil {
		writeValidationError(w, verr)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse_failed", err.Error())
		return
	}
	if h.deps.state == nil {
		writeError(w, http.StatusInternalServerError, "no_state", "")
		return
	}
	if err := h.deps.state.SaveProposed(pid, m, string(body)); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "proposed", "project_id": pid})
}

// manifestsHandler returns the live and proposed manifests (raw YAML, or null)
// for a project. Agent-callable: read-only situational awareness that cannot
// change the running stack.
type manifestsHandler struct{ deps applyDeps }

func newManifestsHandler(deps applyDeps) http.Handler { return &manifestsHandler{deps} }

func (h *manifestsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pid := pathVar(r, "pid")
	if pid == "" {
		writeError(w, http.StatusBadRequest, "missing_project_id", "")
		return
	}
	var live, proposed *string
	if h.deps.state != nil {
		if rec, ok := h.deps.state.Get(pid); ok {
			if rec.ManifestYAML != "" {
				v := rec.ManifestYAML
				live = &v
			}
			if rec.ProposedYAML != "" {
				v := rec.ProposedYAML
				proposed = &v
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"live": live, "proposed": proposed})
}
