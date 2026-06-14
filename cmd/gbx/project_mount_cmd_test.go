package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/projectid"
)

// setupMountTestProject creates a temp config dir with one registered project
// pointing at a real tempdir workspace and returns a fresh host directory
// suitable as a mount source. The temp config dir is wired in via t.Setenv;
// the project pid is derivable from the workspace by callers that need it.
func setupMountTestProject(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	ws := t.TempDir()
	pid, err := projectid.Hash(ws)
	if err != nil {
		t.Fatal(err)
	}
	wsResolved, _ := filepath.EvalSymlinks(ws)
	projDir := filepath.Join(cfg, "state", "projects", pid)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "workspace-path"), []byte(wsResolved+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "active-project"), []byte(pid+"\n"+wsResolved+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GBX_CONFIG_DIR", cfg)
	t.Setenv("GBX_STATE_DIR", filepath.Join(cfg, "state"))
	t.Setenv("GBX_OVERRIDE_PID", "")
	_ = pid
	return t.TempDir()
}

func TestProjectMountAddLsRm(t *testing.T) {
	host := setupMountTestProject(t)

	// add
	stdout, stderr, code := runCLI(t, "mount", "add", host+":/data:ro")
	if code != 0 {
		t.Fatalf("add exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Added ") {
		t.Errorf("add stdout: want 'Added', got %q", stdout)
	}

	// ls shows the entry
	stdout, _, code = runCLI(t, "mount", "ls")
	if code != 0 {
		t.Fatalf("ls exit=%d", code)
	}
	if !strings.Contains(stdout, ":/data:ro") {
		t.Errorf("ls stdout: want '/data:ro', got %q", stdout)
	}

	// duplicate add fails
	_, stderr, code = runCLI(t, "mount", "add", host+":/data:rw")
	if code == 0 {
		t.Fatal("want failure for duplicate container path")
	}
	if !strings.Contains(stderr, "already mounted") {
		t.Errorf("stderr: want 'already mounted', got %q", stderr)
	}

	// rm by container path
	_, stderr, code = runCLI(t, "mount", "rm", "/data")
	if code != 0 {
		t.Fatalf("rm exit=%d stderr=%q", code, stderr)
	}

	// ls is empty again
	stdout, _, _ = runCLI(t, "mount", "ls")
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("ls after rm should be empty, got %q", stdout)
	}

	// rm with no match fails
	_, stderr, code = runCLI(t, "mount", "rm", "/does/not/exist")
	if code == 0 {
		t.Fatal("want failure when nothing matches")
	}
	if !strings.Contains(stderr, "no mount matched") {
		t.Errorf("stderr: want 'no mount matched', got %q", stderr)
	}
}

func TestProjectMountAddRejectsReservedContainer(t *testing.T) {
	host := setupMountTestProject(t)
	_, stderr, code := runCLI(t, "mount", "add", host+":/workspace")
	if code == 0 {
		t.Fatal("want failure for reserved container path")
	}
	if !strings.Contains(stderr, "reserved") {
		t.Errorf("stderr: want 'reserved', got %q", stderr)
	}
}
