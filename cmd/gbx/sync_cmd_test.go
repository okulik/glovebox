package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncAllConflictsWithPid(t *testing.T) {
	t.Setenv("GBX_OVERRIDE_PID", "somepid")
	_, _, code := runCLI(t, "sync", "--all")
	if code == 0 {
		t.Fatal("expected non-zero exit for --all together with -p")
	}
}

func TestSyncAllNoProjects(t *testing.T) {
	t.Setenv("GBX_STATE_DIR", t.TempDir()) // no projects/ subdir
	stdout, _, code := runCLI(t, "sync", "--all")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "No projects to sync") {
		t.Fatalf("stdout: %q", stdout)
	}
}

func TestSyncSingleProjectSeeds(t *testing.T) {
	base := t.TempDir()
	libexec := filepath.Join(base, "libexec")
	defClaude := filepath.Join(libexec, "defaults", "claude")
	if err := os.MkdirAll(defClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defClaude, "settings.json"),
		[]byte(`{"model":"sonnet","permissions":{"allow":["Bash(gbx-stack *)"]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defClaude, "statusline-command.sh"),
		[]byte("#!/bin/bash\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libexec, "defaults", "agent-instructions.md"),
		[]byte("summary text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(base, "state")
	pid := "p1"
	if err := os.MkdirAll(filepath.Join(stateDir, "projects", pid), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GBX_STATE_DIR", stateDir)
	t.Setenv("GBX_LIBEXEC", libexec)
	t.Setenv("GBX_OVERRIDE_PID", pid)

	stdout, _, code := runCLI(t, "sync")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, pid) {
		t.Fatalf("stdout missing pid: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "projects", pid, "claude", "settings.json")); err != nil {
		t.Errorf("settings.json not seeded: %v", err)
	}
	if !strings.Contains(stdout, "next session") {
		t.Errorf("expected next-session note in stdout: %q", stdout)
	}
}
