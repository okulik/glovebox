package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func withPathVar(ctx context.Context, k, v string) context.Context {
	return context.WithValue(ctx, pathVarKey(k), v)
}

func defaultTestRules() manifest.Rules {
	return manifest.Rules{
		AllowedRegistries: map[string]struct{}{"docker.io": {}},
		AllowedEnvVars:    map[string]struct{}{},
		MaxCPUs:           4,
		MaxMemoryBytes:    8 << 30,
	}
}

func TestApply_RejectsBadManifest(t *testing.T) {
	h := newApplyHandler(applyDeps{
		rules:    defaultTestRules(),
		hostOnly: true,
	})
	yml := []byte("version: 1\nservices:\n  x:\n    image: sketchy.example.com/foo:1.0\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "image_registry_not_allowed" {
		t.Errorf("error = %q, want image_registry_not_allowed; body = %v", body["error"], body)
	}
}

func TestApply_RejectsFromInternalListener(t *testing.T) {
	h := newApplyHandler(applyDeps{hostOnly: false})
	yml := []byte("version: 1\nservices:\n  x:\n    image: docker.io/library/redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 host_only, got %d", w.Code)
	}
}

func TestApply_CreatesNetworkAndPullsImages(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{
		"id-glovebox-stack-p1-redis": {"healthy"},
		"id-glovebox-stack-p1-neo4j": {"healthy"},
	}
	h := newApplyHandler(applyDeps{rules: defaultTestRules(), docker: fake, hostOnly: true})
	yml := []byte(`
version: 1
services:
  redis:
    image: redis:7-alpine
  neo4j:
    image: neo4j:5.20
`)
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !fake.Networks["glovebox-stack-p1"] {
		t.Errorf("network not created")
	}
	if len(fake.PulledImages) != 2 {
		t.Errorf("pulled = %v", fake.PulledImages)
	}
	if len(fake.Containers) != 2 {
		t.Errorf("containers = %d", len(fake.Containers))
	}
}

func TestApply_WaitsForHealthyAndReturnsOK(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{
		"id-glovebox-stack-p1-redis": {"starting", "healthy"},
	}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestApply_AttachesAgentToStackNetworkOnSuccess(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-agent-p1"] = dockerx.FakeContainer{ID: "id-glovebox-agent-p1", State: "running"}
	fake.HealthSeq = map[string][]string{"id-glovebox-stack-p1-redis": {"healthy"}}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	// The agent (NOT the redis service container) gets one ConnectNetwork
	// call against the project's stack network. Service containers are
	// attached via CreateContainer, not via ConnectNetwork.
	if len(fake.NetworkConnects) != 1 {
		t.Fatalf("expected exactly 1 ConnectNetwork call, got %d (%v)", len(fake.NetworkConnects), fake.NetworkConnects)
	}
	got := fake.NetworkConnects[0]
	if got.Container != "glovebox-agent-p1" || got.Network != "glovebox-stack-p1" {
		t.Errorf("attach got (%q, %q), want (glovebox-agent-p1, glovebox-stack-p1)", got.Container, got.Network)
	}
}

func TestApply_SkipsAgentAttachWhenAgentMissing(t *testing.T) {
	fake := dockerx.NewFake()
	// No glovebox-agent-p1 in fake.Containers.
	fake.HealthSeq = map[string][]string{"id-glovebox-stack-p1-redis": {"healthy"}}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("apply must still succeed when agent missing; status = %d body = %s", w.Code, w.Body.String())
	}
	if len(fake.NetworkConnects) != 0 {
		t.Errorf("expected no attach when agent missing, got %v", fake.NetworkConnects)
	}
}

func TestApply_RollsBackOnFailedPull(t *testing.T) {
	fake := dockerx.NewFake()
	fake.PullErr = map[string]error{"neo4j:5.20": errors.New("boom")}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte(`
version: 1
services:
  redis:
    image: redis:7-alpine
  neo4j:
    image: neo4j:5.20
`)
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code == http.StatusOK {
		t.Fatal("expected failure")
	}
	if fake.Networks["glovebox-stack-p1"] {
		t.Errorf("network not rolled back")
	}
	if len(fake.Containers) != 0 {
		t.Errorf("containers leaked: %+v", fake.Containers)
	}
}

func TestApply_TimesOutWhenNeverHealthy(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{
		"id-glovebox-stack-p1-redis": {"starting", "starting", "starting", "starting", "starting"},
	}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthTimeout: 50 * time.Millisecond, healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestApply_PerProjectMutexSerializes(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{"id-glovebox-stack-p1-redis": {"healthy", "healthy"}}
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")

	done := make(chan int, 2)
	for range 2 {
		go func() {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
			done <- w.Code
		}()
	}
	for range 2 {
		if code := <-done; code != http.StatusOK {
			t.Errorf("apply returned %d, want 200", code)
		}
	}

	// Both applies succeeded but the mutex serialized them, so the second is a
	// no-op: exactly one pull and one container, not a racing duplicate.
	if len(fake.PulledImages) != 1 {
		t.Errorf("expected 1 pull, got %v", fake.PulledImages)
	}
	if len(fake.Containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(fake.Containers))
	}
}

func TestApply_FromStoredProposal(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{"id-glovebox-stack-p1-redis": {"healthy"}}
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	yml := "version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n"
	_ = s.SaveProposed("p1", m, yml)

	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, state: s, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	// Empty body ⇒ apply the stored proposal.
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	rec, ok := s.Get("p1")
	if !ok || rec.Manifest == nil {
		t.Fatalf("live manifest not saved: %+v", rec)
	}
	if rec.ManifestYAML != yml {
		t.Errorf("ManifestYAML = %q", rec.ManifestYAML)
	}
	if rec.Proposed != nil || rec.ProposedYAML != "" {
		t.Errorf("proposal not cleared after apply: %+v", rec)
	}
}

func TestApply_EmptyBodyNoProposal(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	h := newApplyHandler(applyDeps{rules: defaultTestRules(), docker: dockerx.NewFake(), state: s, hostOnly: true})
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "no_proposal" {
		t.Errorf("error = %q, want no_proposal", body["error"])
	}
}

func TestApply_BodyApplyClearsStaleProposal(t *testing.T) {
	fake := dockerx.NewFake()
	fake.HealthSeq = map[string][]string{"id-glovebox-stack-p1-redis": {"healthy"}}
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	m := &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}
	_ = s.SaveProposed("p1", m, "STALE")
	h := newApplyHandler(applyDeps{
		rules: defaultTestRules(), docker: fake, state: s, hostOnly: true,
		healthInterval: 5 * time.Millisecond,
	})
	yml := []byte("version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n")
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects/p1/apply", bytes.NewReader(yml))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(withPathVar(r.Context(), "pid", "p1")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	rec, _ := s.Get("p1")
	if rec.Proposed != nil {
		t.Errorf("stale proposal not cleared")
	}
}
