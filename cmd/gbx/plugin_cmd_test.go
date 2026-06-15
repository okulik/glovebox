package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedProject creates a minimal project state dir and marks it active so
// targetPID() resolves to it. Returns the pid.
func seedProject(t *testing.T, cfg string) string {
	t.Helper()
	pid := "abc123def456"
	projDir := filepath.Join(cfg, "state", "projects", pid)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "workspace-path"), []byte("/tmp/ws"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "active-project"), []byte(pid), 0o644); err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestPluginAddStoresFragment(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("GBX_CONFIG_DIR", cfg)
	t.Setenv("GBX_STATE_DIR", filepath.Join(cfg, "state"))
	pid := seedProject(t, cfg)

	prev := launchEditor
	t.Cleanup(func() { launchEditor = prev })
	launchEditor = func(path string) error {
		return os.WriteFile(path, []byte("# gbx:description: my plugin\nRUN true\n"), 0o644)
	}

	stdout, stderr, code := runCLI(t, "plugin", "add")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Run `gbx rebuild`") {
		t.Errorf("missing rebuild hint: %q", stdout)
	}
	pluginsDir := filepath.Join(cfg, "state", "projects", pid, "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want exactly 1 stored plugin, got %d", count)
	}
}

func TestPluginAddRejectsMissingDescriptionAndKeepsDraft(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("GBX_CONFIG_DIR", cfg)
	t.Setenv("GBX_STATE_DIR", filepath.Join(cfg, "state"))
	pid := seedProject(t, cfg)

	prev := launchEditor
	t.Cleanup(func() { launchEditor = prev })
	launchEditor = func(path string) error {
		return os.WriteFile(path, []byte("RUN true\n"), 0o644) // no description
	}

	_, stderr, code := runCLI(t, "plugin", "add")
	if code == 0 {
		t.Fatal("want non-zero exit for missing description")
	}
	if !strings.Contains(stderr, "description") {
		t.Errorf("stderr should mention description: %q", stderr)
	}
	// The draft must be preserved so the user doesn't lose their work, and the
	// path must be reported on stderr.
	if !strings.Contains(stderr, "Draft kept at") {
		t.Errorf("stderr should report the kept draft path: %q", stderr)
	}
	pluginsDir := filepath.Join(cfg, "state", "projects", pid, "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	drafts := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".draft-") {
			drafts++
		}
	}
	if drafts != 1 {
		t.Errorf("want exactly 1 kept draft, got %d (entries=%v)", drafts, entries)
	}
}

func TestPluginLsAndRm(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("GBX_CONFIG_DIR", cfg)
	t.Setenv("GBX_STATE_DIR", filepath.Join(cfg, "state"))
	pid := seedProject(t, cfg)

	pluginsDir := filepath.Join(cfg, "state", "projects", pid, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "dead0000"
	if err := os.WriteFile(filepath.Join(pluginsDir, id),
		[]byte("# gbx:description: listed plugin\nRUN true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "plugin", "ls")
	if code != 0 {
		t.Fatalf("ls exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, id) || !strings.Contains(stdout, "listed plugin") {
		t.Errorf("ls output missing id/description: %q", stdout)
	}

	_, stderr, code = runCLI(t, "plugin", "rm", id, "-y")
	if code != 0 {
		t.Fatalf("rm exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(pluginsDir, id)); !os.IsNotExist(err) {
		t.Error("plugin file should be removed")
	}
}
