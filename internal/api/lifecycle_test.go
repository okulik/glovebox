package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func TestStatus_NoStack_ReturnsDown(t *testing.T) {
	fake := dockerx.NewFake()
	h := newStatusHandler(applyDeps{docker: fake})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/empty/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "empty")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["state"] != "down" {
		t.Errorf("state = %v", body["state"])
	}
}

func TestListProjects_RequiresHostOnly(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	_ = s.Save("p1", &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}, "applied", "")
	h := newListProjectsHandler(applyDeps{state: s, hostOnly: true})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListProjects_RejectsFromInternal(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	h := newListProjectsHandler(applyDeps{state: s, hostOnly: false})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDown_StopsServices_KeepsVolumes(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-p1-redis"] = dockerx.FakeContainer{ID: "id-redis", State: "running", Health: "healthy"}
	fake.Volumes["glovebox-stack-p1-redis-data"] = true
	h := newDownHandler(applyDeps{docker: fake, hostOnly: true})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/down", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if fake.Containers["glovebox-stack-p1-redis"].State == "running" {
		t.Errorf("redis not stopped")
	}
	if !fake.Volumes["glovebox-stack-p1-redis-data"] {
		t.Errorf("volume should be kept")
	}
}

func TestDestroy_RequiresConfirm(t *testing.T) {
	h := newDestroyHandler(applyDeps{docker: dockerx.NewFake(), hostOnly: true})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/destroy", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDestroy_RemovesEverything(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-p1-redis"] = dockerx.FakeContainer{ID: "id1", State: "running"}
	fake.Volumes["glovebox-stack-p1-redis-data"] = true
	fake.Networks["glovebox-stack-p1"] = true
	h := newDestroyHandler(applyDeps{docker: fake, hostOnly: true})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/destroy?confirm=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if len(fake.Containers) != 0 || len(fake.Volumes) != 0 || fake.Networks["glovebox-stack-p1"] {
		t.Errorf("leaked: %+v / %+v / %v", fake.Containers, fake.Volumes, fake.Networks)
	}
}

func TestStatus_RunningServicesReadsAsReady(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-stack-p1-redis"] = dockerx.FakeContainer{ID: "id1", State: "running", Health: "healthy"}
	h := newStatusHandler(applyDeps{docker: fake})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/p1/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatal("status non-200")
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["state"] != "ready" {
		t.Errorf("state = %v", body["state"])
	}
	svcs := body["services"].(map[string]any)
	if _, ok := svcs["redis"]; !ok {
		t.Errorf("services = %+v", svcs)
	}
}
