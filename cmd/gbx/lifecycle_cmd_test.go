package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

// setupRebuildEnv wires fake docker + a stubbed ensureAgentFn and a project
// state dir, returning (cfg, pid).
func setupRebuildEnv(t *testing.T) (string, string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv(config.EnvConfigDir, cfg)
	t.Setenv(config.EnvStateDir, filepath.Join(cfg, config.StatePath))
	t.Setenv(config.EnvLibexec, t.TempDir())
	if err := os.WriteFile(filepath.Join(cfg, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid := "abc123def456"
	projDir := filepath.Join(cfg, config.StatePath, config.ProjectsPath, pid)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, config.WorkspacePath), []byte("/tmp/ws"), 0o644); err != nil {
		t.Fatal(err)
	}

	fh := dockerx.NewFakeHost()
	hostDocker = fh
	hostClient = dockerx.NewFake()
	prevEnsure := ensureAgentFn
	t.Cleanup(func() { hostDocker = nil; hostClient = nil; ensureAgentFn = prevEnsure })
	ensureAgentFn = func(_ context.Context, _ dockerx.ControllerClient, _, _, _, _ string) error { return nil }
	return cfg, pid
}

func TestRebuildBuildsDerivedImageWhenPluginsPresent(t *testing.T) {
	cfg, pid := setupRebuildEnv(t)
	pluginsDir := filepath.Join(cfg, config.StatePath, config.ProjectsPath, pid, config.PluginsPath)
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "dead0000"),
		[]byte("# gbx:description: x\nRUN true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "rebuild", pid)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	fh := hostDocker.(*dockerx.FakeHost)
	wantTag := config.ContainerAgentPrefix + pid + ":local"
	foundBuild := false
	for _, c := range fh.Calls {
		if strings.HasPrefix(c, "build "+wantTag+" ") {
			foundBuild = true
		}
	}
	if !foundBuild {
		t.Errorf("expected a build of %s; calls=%v", wantTag, fh.Calls)
	}
	if _, err := os.Stat(filepath.Join(cfg, config.StatePath, config.ProjectsPath, pid, "Dockerfile.plugins")); err != nil {
		t.Errorf("Dockerfile.plugins not written: %v", err)
	}
}

func TestRebuildRemovesStaleDerivedImageWhenNoPlugins(t *testing.T) {
	_, pid := setupRebuildEnv(t)
	fh := hostDocker.(*dockerx.FakeHost)
	derived := config.ContainerAgentPrefix + pid + ":local"
	fh.Images[derived] = true

	_, stderr, code := runCLI(t, "rebuild", pid)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if fh.Images[derived] {
		t.Error("stale derived image should have been removed")
	}
	foundRmi := false
	for _, c := range fh.Calls {
		if c == "rmi "+derived {
			foundRmi = true
		}
	}
	if !foundRmi {
		t.Errorf("expected rmi %s; calls=%v", derived, fh.Calls)
	}
}

// TestRebuildAllBuildsDerivedForPluginProject exercises the --all branch (the
// state/projects scan) end to end: a plugin'd project discovered via --all
// still gets its derived image built. Also guards against a regression in the
// projectsDir/projectsPath naming used by that branch.
func TestRebuildAllBuildsDerivedForPluginProject(t *testing.T) {
	cfg, pid := setupRebuildEnv(t)
	pluginsDir := filepath.Join(cfg, config.StatePath, config.ProjectsPath, pid, config.PluginsPath)
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "dead0000"),
		[]byte("# gbx:description: x\nRUN true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "rebuild", "--all")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	fh := hostDocker.(*dockerx.FakeHost)
	wantTag := config.ContainerAgentPrefix + pid + ":local"
	foundBuild := false
	for _, c := range fh.Calls {
		if strings.HasPrefix(c, "build "+wantTag+" ") {
			foundBuild = true
		}
	}
	if !foundBuild {
		t.Errorf("expected --all to build %s; calls=%v", wantTag, fh.Calls)
	}
}
