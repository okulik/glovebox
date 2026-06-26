package state_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func TestReconcile_RestartsMissingContainers(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	_ = s.Save("p1", &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}, "applied", "")

	fake := dockerx.NewFake()
	if err := state.Reconcile(context.Background(), s, fake); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Containers["glovebox-stack-p1-redis"]; !ok {
		t.Errorf("reconcile should have created redis")
	}
}

func TestReconcile_AttachesAgentToStackNetwork(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	_ = s.Save("p1", &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}, "applied", "")

	fake := dockerx.NewFake()
	// Pretend the per-project agent container already exists (compose started it).
	fake.Containers["glovebox-agent-p1"] = dockerx.FakeContainer{ID: "id-glovebox-agent-p1", State: string(container.StateRunning)}

	if err := state.Reconcile(context.Background(), s, fake); err != nil {
		t.Fatal(err)
	}
	if len(fake.NetworkConnects) != 1 {
		t.Fatalf("expected 1 ConnectNetwork call, got %d (%v)", len(fake.NetworkConnects), fake.NetworkConnects)
	}
	got := fake.NetworkConnects[0]
	if got.Container != "glovebox-agent-p1" || got.Network != "glovebox-stack-p1" {
		t.Errorf("attach got (%q, %q), want (glovebox-agent-p1, glovebox-stack-p1)", got.Container, got.Network)
	}
}

func TestReconcile_SkipsAgentAttachWhenAgentMissing(t *testing.T) {
	s, _ := state.Open(filepath.Join(t.TempDir(), "p.json"))
	_ = s.Save("p1", &manifest.Manifest{Version: 1, Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}}}, "applied", "")

	fake := dockerx.NewFake()

	if err := state.Reconcile(context.Background(), s, fake); err != nil {
		t.Fatal(err)
	}
	if len(fake.NetworkConnects) != 0 {
		t.Errorf("expected no attach when agent missing, got %v", fake.NetworkConnects)
	}
}
