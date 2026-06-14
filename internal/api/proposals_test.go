package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func TestPropose_StoresValidManifest(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	// hostOnly:false → propose is agent-callable; it must still work.
	h := newProposeHandler(applyDeps{rules: defaultTestRules(), state: s, hostOnly: false})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: docker.io/library/redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/propose", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	rec, ok := s.Get("p1")
	if !ok || rec.Proposed == nil {
		t.Fatalf("proposal not stored: %+v", rec)
	}
	if string(yml) != rec.ProposedYAML {
		t.Errorf("ProposedYAML = %q", rec.ProposedYAML)
	}
}

func TestPropose_RejectsBadManifest(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := newProposeHandler(applyDeps{rules: defaultTestRules(), state: s})
	yml := []byte("version: 1\nservices:\n  x:\n    image: sketchy.example.com/foo:1.0\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/propose", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	if _, ok := s.Get("p1"); ok {
		t.Errorf("invalid proposal must not be stored")
	}
}

func TestManifests_ReturnsLiveAndProposed(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	_ = s.SaveApplied("p1", m, "LIVE_YAML", "applied", "")
	_ = s.SaveProposed("p1", m, "PROPOSED_YAML")
	h := newManifestsHandler(applyDeps{state: s})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/p1/manifests", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Live     *string `json:"live"`
		Proposed *string `json:"proposed"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Live == nil || *body.Live != "LIVE_YAML" {
		t.Errorf("live = %v", body.Live)
	}
	if body.Proposed == nil || *body.Proposed != "PROPOSED_YAML" {
		t.Errorf("proposed = %v", body.Proposed)
	}
}

func TestManifests_NullsWhenAbsent(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := newManifestsHandler(applyDeps{state: s})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/projects/none/manifests", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "none")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Live     *string `json:"live"`
		Proposed *string `json:"proposed"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Live != nil || body.Proposed != nil {
		t.Errorf("expected both null, got live=%v proposed=%v", body.Live, body.Proposed)
	}
}
