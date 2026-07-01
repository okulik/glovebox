package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/dockerx"
)

func baseSpec(t *testing.T, proj string) agent.EnsureSpec {
	t.Helper()
	return agent.EnsureSpec{
		Create: agent.CreateSpec{
			PID:            "aaaa1111bbbb",
			Workspace:      filepath.Join(proj, "ws"),
			Image:          "glovebox-agent:local",
			StateProjDir:   filepath.Join(proj, "state", "projects", "aaaa1111bbbb"),
			StateSharedDir: filepath.Join(proj, "state", "shared"),
			DockerDir:      filepath.Join(proj, "libexec", "docker"),
			HostEnv:        map[string]string{},
		},
	}
}

func TestEnsureCreatesAndStartsWhenContainerAbsent(t *testing.T) {
	proj := t.TempDir()
	spec := baseSpec(t, proj)
	fake := dockerx.NewFake()
	spec.Docker = fake

	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	cname := "glovebox-agent-aaaa1111bbbb"
	c, ok := fake.Containers[cname]
	if !ok {
		t.Fatalf("container not created: %+v", fake.Containers)
	}
	if c.State != string(container.StateRunning) {
		t.Errorf("container state = %q, want running", c.State)
	}
	if c.Image != "glovebox-agent:local" {
		t.Errorf("container image = %q", c.Image)
	}
	for _, sub := range agent.Names {
		if _, err := os.Stat(filepath.Join(spec.Create.StateProjDir, sub)); err != nil {
			t.Errorf("subdir not created: %s: %v", sub, err)
		}
	}
}

func TestEnsureSkipsCreateWhenContainerExists(t *testing.T) {
	proj := t.TempDir()
	spec := baseSpec(t, proj)
	fake := dockerx.NewFake()
	spec.Docker = fake

	// Pre-seed: container exists in "exited" state. Ensure should start it
	// without replacing the prior FakeContainer record.
	cname := "glovebox-agent-aaaa1111bbbb"
	fake.Containers[cname] = dockerx.FakeContainer{
		ID: "pre-existing-id", Image: "old-image", State: string(container.StateExited),
	}
	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	c := fake.Containers[cname]
	if c.ID != "pre-existing-id" {
		t.Errorf("Ensure should NOT have recreated the container: ID=%q", c.ID)
	}
	if c.State != string(container.StateRunning) {
		t.Errorf("Ensure should have started the existing container: state=%q", c.State)
	}
}

func TestEnsureAttachesStackNetworkIfExists(t *testing.T) {
	proj := t.TempDir()
	spec := baseSpec(t, proj)
	fake := dockerx.NewFake()
	spec.Docker = fake
	// Stack network exists.
	fake.NetworkContainers["glovebox-stack-aaaa1111bbbb"] = 0

	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	gotConnect := false
	for _, nc := range fake.NetworkConnects {
		if nc.Container == "glovebox-agent-aaaa1111bbbb" && nc.Network == "glovebox-stack-aaaa1111bbbb" {
			gotConnect = true
		}
	}
	if !gotConnect {
		t.Errorf("expected ConnectNetwork to be recorded; got %+v", fake.NetworkConnects)
	}
}

func TestEnsureDoesNotAttachWhenStackNetworkAbsent(t *testing.T) {
	proj := t.TempDir()
	spec := baseSpec(t, proj)
	fake := dockerx.NewFake()
	spec.Docker = fake
	// NetworkContainers has no entry → exists=false → no Connect call.

	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(fake.NetworkConnects) != 0 {
		t.Errorf("no ConnectNetwork expected, got %+v", fake.NetworkConnects)
	}
}

func TestEnsureSeedsClaudeDefaultsOnFirstCreate(t *testing.T) {
	proj := t.TempDir()
	// Stage <libexec>/defaults/claude/{settings.json,statusline-command.sh}
	// the way the real repo layout ships them.
	defaultsClaude := filepath.Join(proj, "libexec", "defaults", "claude")
	if err := os.MkdirAll(defaultsClaude, 0o755); err != nil {
		t.Fatalf("mkdir defaults: %v", err)
	}
	settingsPayload := []byte(`{"model":"sonnet"}`)
	statuslinePayload := []byte("#!/bin/sh\necho hi\n")
	if err := os.WriteFile(filepath.Join(defaultsClaude, "settings.json"), settingsPayload, 0o644); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultsClaude, "statusline-command.sh"), statuslinePayload, 0o644); err != nil {
		t.Fatalf("seed statusline-command.sh: %v", err)
	}
	spec := baseSpec(t, proj)
	spec.Docker = dockerx.NewFake()
	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	claudeDir := filepath.Join(spec.Create.StateProjDir, "claude")
	got, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not seeded: %v", err)
	}
	if string(got) != string(settingsPayload) {
		t.Errorf("settings.json mismatch: got %q, want %q", got, settingsPayload)
	}
	st, err := os.Stat(filepath.Join(claudeDir, "statusline-command.sh"))
	if err != nil {
		t.Fatalf("statusline-command.sh not seeded: %v", err)
	}
	if st.Mode().Perm()&0o111 == 0 {
		t.Errorf("statusline-command.sh not executable, mode=%v", st.Mode())
	}
}

func TestEnsurePreservesUserEditsToClaudeDefaults(t *testing.T) {
	proj := t.TempDir()
	defaultsClaude := filepath.Join(proj, "libexec", "defaults", "claude")
	if err := os.MkdirAll(defaultsClaude, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultsClaude, "settings.json"), []byte(`{"model":"sonnet"}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pre-seed the per-project claude dir with user-edited content.
	claudeDir := filepath.Join(proj, "state", "projects", "aaaa1111bbbb", "claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	userEdited := []byte(`{"model":"opus","custom":true}`)
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), userEdited, 0o644); err != nil {
		t.Fatalf("seed user settings: %v", err)
	}
	spec := baseSpec(t, proj)
	fake := dockerx.NewFake()
	spec.Docker = fake
	// Pretend the container already exists so Ensure skips create.
	fake.Containers["glovebox-agent-aaaa1111bbbb"] = dockerx.FakeContainer{
		ID: "pre", State: string(container.StateExited),
	}
	if err := os.MkdirAll(spec.Create.Workspace, 0o755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	if err := agent.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if string(got) != string(userEdited) {
		t.Errorf("user-edited settings.json was overwritten: got %q, want %q", got, userEdited)
	}
}

func TestRemoveDeletesContainer(t *testing.T) {
	fake := dockerx.NewFake()
	fake.Containers["glovebox-agent-aaaa1111bbbb"] = dockerx.FakeContainer{ID: "x", State: string(container.StateRunning)}
	if err := agent.Remove(context.Background(), fake, "aaaa1111bbbb"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := fake.Containers["glovebox-agent-aaaa1111bbbb"]; ok {
		t.Error("container should be gone after Remove")
	}
}

func TestRemoveTolerantOfMissingContainer(t *testing.T) {
	fake := dockerx.NewFake()
	if err := agent.Remove(context.Background(), fake, "no-such-pid"); err != nil {
		t.Errorf("Remove on missing container should not error: %v", err)
	}
}
