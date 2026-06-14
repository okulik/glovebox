package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func newTestStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

func TestServiceStart_RejectsUndeclaredService(t *testing.T) {
	// No manifest stored.
	store := newTestStore(t)
	deps := applyDeps{docker: dockerx.NewFake(), state: store}
	h := newServiceHandler(deps, "start")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1-nostack/services/redis/start", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(withPathVar(r.Context(), "pid", "p1-nostack"), "svc", "redis")))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServiceReset_WipesVolumesAndRestarts(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-pReset-redis"] = dockerx.FakeContainer{ID: "id1", State: "running"}
	fake.Volumes["glovebox-stack-pReset-redis-data"] = true
	store := newTestStore(t)
	_ = store.Save("pReset", &manifest.Manifest{
		Version:  1,
		Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine", Volumes: map[string]string{"data": "/data"}}},
	}, "applied", "")
	h := newResetHandler(applyDeps{docker: fake, state: store})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/pReset/services/redis/reset", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(withPathVar(r.Context(), "pid", "pReset"), "svc", "redis")))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !fake.Volumes["glovebox-stack-pReset-redis-data"] {
		t.Errorf("data volume should exist after reset")
	}
	// New container should have a different ID than "id1".
	if fake.Containers["glovebox-stack-pReset-redis"].ID == "id1" {
		t.Errorf("container was not recreated")
	}
}

func TestServiceReset_LeavesVolumesPresentIfRecreateFails(t *testing.T) {
	// Setup: a service that is currently up, plus a manifest in the store.
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-pBust-redis"] = dockerx.FakeContainer{ID: "id-old", State: "running"}
	fake.Volumes["glovebox-stack-pBust-redis-data"] = true
	// Force CreateContainer to fail so we exercise the failure path.
	fake.CreateErr["glovebox-stack-pBust-redis"] = errors.New("synthetic create failure")

	store := newTestStore(t)
	_ = store.Save("pBust", &manifest.Manifest{
		Version:  1,
		Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine", Volumes: map[string]string{"data": "/data"}}},
	}, "applied", "")

	h := newResetHandler(applyDeps{docker: fake, state: store})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/pBust/services/redis/reset", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(withPathVar(r.Context(), "pid", "pBust"), "svc", "redis")))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	// The invariant: even though recreate failed, the data volume must exist
	// (empty, since reset wipes it) so a follow-up `gbx stack apply` can
	// recover by re-creating the container against it.
	if !fake.Volumes["glovebox-stack-pBust-redis-data"] {
		t.Errorf("volume must exist after failed reset so re-apply can recover")
	}
}

func TestInfo_ReturnsServiceMap(t *testing.T) {
	store := newTestStore(t)
	_ = store.Save("pInfo", &manifest.Manifest{
		Version: 1, Services: map[string]manifest.Service{
			"redis": {Image: "redis:7-alpine"},
			"neo4j": {Image: "neo4j:5.20"},
		},
	}, "applied", "")
	h := newInfoHandler(applyDeps{state: store})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/pInfo/info", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "pInfo")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	services, _ := body["services"].(map[string]any)
	if _, ok := services["redis"]; !ok {
		t.Errorf("missing redis: %v", services)
	}
	if _, ok := services["neo4j"]; !ok {
		t.Errorf("missing neo4j")
	}
}

func TestLogs_404WhenNoManifest(t *testing.T) {
	store := newTestStore(t)
	h := newLogsHandler(applyDeps{docker: dockerx.NewFake(), state: store})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/pLog/services/redis/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(withPathVar(r.Context(), "pid", "pLog"), "svc", "redis")))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServiceStart_StartsDeclaredService(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-pStart-redis"] = dockerx.FakeContainer{ID: "id1", State: "exited"}
	store := newTestStore(t)
	deps := applyDeps{docker: fake, state: store}
	_ = store.Save("pStart", &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}, "applied", "")
	h := newServiceHandler(deps, "start")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/pStart/services/redis/start", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(withPathVar(r.Context(), "pid", "pStart"), "svc", "redis")))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if fake.Containers["glovebox-stack-pStart-redis"].State != "running" {
		t.Errorf("not running: %+v", fake.Containers)
	}
}
