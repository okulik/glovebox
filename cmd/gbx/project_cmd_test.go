package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

func TestProjectUse(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath, "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.StatePath, config.ProjectsPath, "aaaa1111bbbb", config.WorkspacePath),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	stdout, stderr, code := runCLI(t, "use", "aaaa")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Default project:") {
		t.Fatalf("stdout: want 'Default project:', got %q", stdout)
	}
	data, _ := os.ReadFile(filepath.Join(dir, config.ActiveProjectPath))
	if !strings.HasPrefix(string(data), "aaaa1111bbbb") {
		t.Fatalf("active-project not written: %q", data)
	}
}

func TestProjectUseUnknownPid(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	_, stderr, code := runCLI(t, "use", "deadbeefdead")
	if code == 0 {
		t.Fatal("want non-zero exit for unknown pid")
	}
	if !strings.Contains(stderr, "No project matches") {
		t.Fatalf("stderr: want 'No project matches', got %q", stderr)
	}
}

func TestProjectLsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	stdout, _, code := runCLI(t, "ls")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("want only header line, got %d lines: %q", len(lines), stdout)
	}
}

// seedLsProject creates one registered project plus a fake docker pool with
// the project's agent container and one singleton-stack ("system") container,
// then injects the fake as the package-level hostClient for the test.
func seedLsProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	pid := "aaaa1111bbbb"
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath, pid), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.StatePath, config.ProjectsPath, pid, config.WorkspacePath),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	fake := dockerx.NewFake()
	fake.Containers[config.ContainerAgentPrefix+pid] = dockerx.FakeContainer{
		ID: "id-agent", Image: "glovebox-agent:local", State: string(container.StateRunning), Status: "Up 2 hours",
	}
	fake.Containers[config.ContainerEgressProxy] = dockerx.FakeContainer{
		ID: "id-proxy", Image: "glovebox-proxy:local", State: string(container.StateRunning), Status: "Up 5 hours",
	}
	hostClient = fake
	t.Cleanup(func() { hostClient = nil })
}

func TestProjectLsVerboseShowsImage(t *testing.T) {
	seedLsProject(t)
	stdout, stderr, code := runCLI(t, "ls", "-v")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "glovebox-agent:local") {
		t.Errorf("agent container image missing: %q", stdout)
	}
	if !strings.Contains(stdout, "glovebox-proxy:local") {
		t.Errorf("system (OTHER) container image missing: %q", stdout)
	}
	if strings.Contains(stdout, "LABELS") {
		t.Errorf("raw LABELS sub-header should be gone: %q", stdout)
	}
}

func TestProjectLsVerboseTree(t *testing.T) {
	seedLsProject(t)
	pid := "aaaa1111bbbb" // the fixed pid seedLsProject registers
	// Give the agent container a build-stamp label so the derived tag shows.
	fake := hostClient.(*dockerx.Fake)
	ac := fake.Containers[config.ContainerAgentPrefix+pid]
	ac.Labels = map[string]string{config.LabelImageCreated: time.Now().UTC().Add(-2 * time.Hour).Format(dockerx.ImageCreatedLabelFormat)}
	fake.Containers[config.ContainerAgentPrefix+pid] = ac

	stdout, stderr, code := runCLI(t, "ls", "-v")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "PROJECTS") {
		t.Errorf("missing PROJECTS heading: %q", stdout)
	}
	if !strings.Contains(stdout, "│ /work/foo") {
		t.Errorf("missing workspace gutter line: %q", stdout)
	}
	if !strings.Contains(stdout, "└─ agent") {
		t.Errorf("missing stripped agent leaf: %q", stdout)
	}
	if !strings.Contains(stdout, "built ") {
		t.Errorf("missing derived built tag: %q", stdout)
	}
	if strings.Contains(stdout, "io.glovebox.") {
		t.Errorf("raw label text should not appear: %q", stdout)
	}
	if !strings.Contains(stdout, "OTHER CONTAINERS") || !strings.Contains(stdout, "egress-proxy") {
		t.Errorf("missing OTHER CONTAINERS section: %q", stdout)
	}
}

func TestProjectLsJSONIncludesImage(t *testing.T) {
	seedLsProject(t)
	stdout, stderr, code := runCLI(t, "ls", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var root jsonRoot
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatalf("ls --json is not valid JSON: %v\n%s", err, stdout)
	}
	if len(root.Projects) != 1 || len(root.Projects[0].Containers) != 1 {
		t.Fatalf("want one project with one container, got %+v", root.Projects)
	}
	if got := root.Projects[0].Containers[0].Image; got != "glovebox-agent:local" {
		t.Errorf("project container image = %q, want glovebox-agent:local", got)
	}
	if len(root.OtherContainers) != 1 || root.OtherContainers[0].Image != "glovebox-proxy:local" {
		t.Errorf("other_containers = %+v, want one with image glovebox-proxy:local", root.OtherContainers)
	}
}

func TestProjectRmYesFullId(t *testing.T) {
	dir := t.TempDir()
	pid := "aaaa1111bbbb"
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath, pid), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.StatePath, config.ProjectsPath, pid, config.WorkspacePath),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	prevRm := removeAgentFn
	t.Cleanup(func() { removeAgentFn = prevRm })
	called := false
	removeAgentFn = func(_ context.Context, _ dockerx.ControllerClient, p string) error {
		called = true
		if p != pid {
			t.Errorf("RemoveAgent called with %q, want %q", p, pid)
		}
		return nil
	}
	_, stderr, code := runCLI(t, "rm", pid, "--delete-state", "--yes")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !called {
		t.Error("RemoveAgent was not called")
	}
	if _, err := os.Stat(filepath.Join(dir, config.StatePath, config.ProjectsPath, pid)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state dir should be removed when --delete-state is passed: %v", err)
	}
}

func TestProjectRmAmbiguousErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath, "abcd11111111"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, config.StatePath, config.ProjectsPath, "abcd22222222"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	_, stderr, code := runCLI(t, "rm", "abcd", "--yes")
	if code == 0 {
		t.Fatal("want non-zero exit for ambiguous")
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Errorf("stderr must mention 'ambiguous', got %q", stderr)
	}
}

func stubLibexec(t *testing.T) string {
	t.Helper()
	libexec := t.TempDir()
	if err := os.WriteFile(filepath.Join(libexec, ".env.example"),
		[]byte("FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libexec, "docker", "proxy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libexec, "docker", "proxy", "allowlist.txt"),
		[]byte("example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return libexec
}

func TestProjectNewRejectsMissingPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GBX_CONFIG_DIR", dir)
	t.Setenv("GBX_STATE_DIR", filepath.Join(dir, config.StatePath))
	t.Setenv("GBX_LIBEXEC", "/tmp/nonexistent-libexec")
	t.Setenv("GBX_SKIP_STACK_UP", "1")
	_, stderr, code := runCLI(t, "new", "/this/does/not/exist/zz")
	if code == 0 {
		t.Fatal("want non-zero exit for missing path")
	}
	if !strings.Contains(stderr, "not a directory") {
		t.Errorf("stderr: want 'not a directory', got %q", stderr)
	}
}

func TestProjectNewHappyPath(t *testing.T) {
	cfg := t.TempDir()
	ws := t.TempDir()
	libexec := stubLibexec(t)
	t.Setenv("GBX_CONFIG_DIR", cfg)
	t.Setenv("GBX_STATE_DIR", filepath.Join(cfg, config.StatePath))
	t.Setenv("GBX_LIBEXEC", libexec)
	t.Setenv("GBX_SKIP_STACK_UP", "1")
	prev := ensureAgentFn
	t.Cleanup(func() { ensureAgentFn = prev })
	called := false
	ensureAgentFn = func(_ context.Context, _ dockerx.ControllerClient, _, _, _, _ string) error {
		called = true
		return nil
	}
	stdout, stderr, code := runCLI(t, "new", ws)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !called {
		t.Error("EnsureAgent was not called")
	}
	if !strings.Contains(stdout, "Set as default.") {
		t.Errorf("stdout: want 'Set as default.', got %q", stdout)
	}
}
