package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/dockerx"
)

type recordedEnsureCall struct {
	PID, Workspace string
}

func newFakeEnsureFn(calls *[]recordedEnsureCall) EnsureAgentFn {
	return func(_ context.Context, _ dockerx.ControllerClient, pid, ws, _, _ string) error {
		*calls = append(*calls, recordedEnsureCall{pid, ws})
		return nil
	}
}

// seedLibexec writes the minimum files hostconfig.Bootstrap needs to seed
// from <libexec>/. Keeps the test-only TempDir wiring out of the call sites.
func seedLibexec(t *testing.T, libexec string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(libexec, ".env.example"),
		[]byte("# example\n"), 0o644); err != nil {
		t.Fatalf("seed .env.example: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(libexec, "docker", "proxy"), 0o755); err != nil {
		t.Fatalf("mkdir docker/proxy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libexec, "docker", "proxy", "allowlist.txt"),
		[]byte(""), 0o644); err != nil {
		t.Fatalf("seed allowlist.txt: %v", err)
	}
}

func TestNewProjectRegistersAndSetsDefaultWhenNoneYet(t *testing.T) {
	cfg := t.TempDir()
	ws := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var calls []recordedEnsureCall
	res, err := New(context.Background(), NewSpec{
		Workspace:   ws,
		ConfigDir:   cfg,
		LibExec:     libexec,
		EnsureAgent: newFakeEnsureFn(&calls),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !res.SetAsDefault {
		t.Error("want SetAsDefault=true on first project")
	}
	projDir := filepath.Join(cfg, "state", "projects", res.PID)
	if _, err := os.Stat(filepath.Join(projDir, "workspace-path")); err != nil {
		t.Errorf("workspace-path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "active-project")); err != nil {
		t.Errorf("active-project missing: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("EnsureAgent should be called once, got %d", len(calls))
	}
}

func TestNewProjectKeepsExistingDefault(t *testing.T) {
	cfg := t.TempDir()
	ws1 := t.TempDir()
	ws2 := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var calls []recordedEnsureCall
	res1, err := New(context.Background(), NewSpec{
		Workspace: ws1, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&calls),
	})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	res2, err := New(context.Background(), NewSpec{
		Workspace: ws2, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&calls),
	})
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if res2.SetAsDefault {
		t.Error("second New should NOT change the default")
	}
	if res2.PID == res1.PID {
		t.Errorf("expected different pids, got %s twice", res1.PID)
	}
	data, _ := os.ReadFile(filepath.Join(cfg, "active-project"))
	if !strings.HasPrefix(string(data), res1.PID) {
		t.Errorf("default changed: %q", data)
	}
}

func TestNewProjectIdempotentOnSecondCall(t *testing.T) {
	cfg := t.TempDir()
	ws := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var calls []recordedEnsureCall
	res1, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&calls),
	})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	res2, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&calls),
	})
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if res1.PID != res2.PID {
		t.Errorf("pid changed: %s vs %s", res1.PID, res2.PID)
	}
	if !res2.AlreadyRegistered {
		t.Error("second New should report AlreadyRegistered=true")
	}
}

func TestNewProjectRejectsMissingPath(t *testing.T) {
	cfg := t.TempDir()
	libexec := t.TempDir()
	_, err := New(context.Background(), NewSpec{
		Workspace: "/this/path/does/not/exist/abcxyz",
		ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&[]recordedEnsureCall{}),
	})
	if err == nil {
		t.Fatal("want error for missing path")
	}
}

func TestUseFlipsDefault(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects", "cccc2222dddd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb", "workspace-path"),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Use(cfg, "aaaa"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(cfg, "active-project"))
	if !strings.HasPrefix(string(data), "aaaa1111bbbb") {
		t.Errorf("default not set: %q", data)
	}
}

func TestUseRejectsUnknownPid(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := Use(cfg, "deadbeefdead")
	if err == nil {
		t.Fatal("want error for unknown pid")
	}
	if !strings.Contains(err.Error(), "No project matches") {
		t.Errorf("error must contain 'No project matches', got %q", err)
	}
}

func TestRemoveAllPaths(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb", "workspace-path"),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "active-project"),
		[]byte("aaaa1111bbbb\n/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}
	removed := []string{}
	fakeRm := func(_ context.Context, _ dockerx.ControllerClient, pid string) error {
		removed = append(removed, pid)
		return nil
	}
	if err := Remove(context.Background(), RemoveSpec{
		Prefix:      "aaaa",
		ConfigDir:   cfg,
		DeleteState: true,
		Confirm:     func() bool { return true },
		RemoveAgent: fakeRm,
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 1 || removed[0] != "aaaa1111bbbb" {
		t.Errorf("agent remove not called as expected: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state dir should be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "active-project")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("active-project should be cleared")
	}
}

func TestRemoveDefaultPreservesStateDir(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb", "workspace-path"),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fakeRm := func(_ context.Context, _ dockerx.ControllerClient, _ string) error { return nil }
	// DeleteState left at its zero value (false) - the new default.
	if err := Remove(context.Background(), RemoveSpec{
		Prefix:      "aaaa",
		ConfigDir:   cfg,
		Confirm:     func() bool { return true },
		RemoveAgent: fakeRm,
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb")); err != nil {
		t.Errorf("state dir should be kept by default: %v", err)
	}
}

func TestRemoveAborts(t *testing.T) {
	cfg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "state", "projects", "aaaa1111bbbb", "workspace-path"),
		[]byte("/work/foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fakeRm := func(_ context.Context, _ dockerx.ControllerClient, _ string) error {
		t.Fatal("RemoveAgent should not be called on abort")
		return nil
	}
	err := Remove(context.Background(), RemoveSpec{
		Prefix:      "aaaa",
		ConfigDir:   cfg,
		Confirm:     func() bool { return false },
		RemoveAgent: fakeRm,
	})
	if err == nil {
		t.Fatal("want error on abort")
	}
}

// Removing a project that was the default must not "resurrect" it as the
// default on the next `project new` against the same workspace path.
func TestNewDoesNotResurrectDefaultAfterRemoval(t *testing.T) {
	cfg := t.TempDir()
	ws := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var ensureCalls []recordedEnsureCall
	first, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if !first.SetAsDefault {
		t.Fatal("first project should have been auto-defaulted")
	}
	if rmErr := Remove(context.Background(), RemoveSpec{
		Prefix:      first.PID,
		ConfigDir:   cfg,
		Confirm:     func() bool { return true },
		RemoveAgent: func(_ context.Context, _ dockerx.ControllerClient, _ string) error { return nil },
	}); rmErr != nil {
		t.Fatalf("Remove: %v", rmErr)
	}
	second, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	})
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if second.PID != first.PID {
		t.Errorf("expected same pid (deterministic from path), got %s then %s", first.PID, second.PID)
	}
	if second.SetAsDefault {
		t.Error("removed-then-recreated project must not auto-default")
	}
	if _, err := os.Stat(filepath.Join(cfg, "active-project")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("active-project should still be absent: %v", err)
	}
}

// A different workspace path created after removing the default must still
// auto-default - the marker is one-shot and pid-specific.
func TestNewAutoDefaultsForDifferentPathAfterRemoval(t *testing.T) {
	cfg := t.TempDir()
	ws1 := t.TempDir()
	ws2 := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var ensureCalls []recordedEnsureCall
	first, err := New(context.Background(), NewSpec{
		Workspace: ws1, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if rmErr := Remove(context.Background(), RemoveSpec{
		Prefix:      first.PID,
		ConfigDir:   cfg,
		Confirm:     func() bool { return true },
		RemoveAgent: func(_ context.Context, _ dockerx.ControllerClient, _ string) error { return nil },
	}); rmErr != nil {
		t.Fatalf("Remove: %v", rmErr)
	}
	second, err := New(context.Background(), NewSpec{
		Workspace: ws2, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	})
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if !second.SetAsDefault {
		t.Error("a different workspace should still auto-default; marker is pid-specific")
	}
}

// `project use` must clear the demote marker so future `project new` calls
// resume normal auto-default behavior even when the workspace path matches
// an earlier-demoted pid.
func TestUseClearsRemovedDefaultMarker(t *testing.T) {
	cfg := t.TempDir()
	ws := t.TempDir()
	libexec := t.TempDir()
	seedLibexec(t, libexec)
	var ensureCalls []recordedEnsureCall
	first, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	})
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if err := Remove(context.Background(), RemoveSpec{
		Prefix:      first.PID,
		ConfigDir:   cfg,
		Confirm:     func() bool { return true },
		RemoveAgent: func(_ context.Context, _ dockerx.ControllerClient, _ string) error { return nil },
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Re-register to populate state, but the auto-default is suppressed.
	if _, err := New(context.Background(), NewSpec{
		Workspace: ws, ConfigDir: cfg, LibExec: libexec,
		EnsureAgent: newFakeEnsureFn(&ensureCalls),
	}); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	// Marker is now consumed (single use). Use the project explicitly.
	if err := Use(cfg, first.PID); err != nil {
		t.Fatalf("Use: %v", err)
	}
	// Subsequent re-register of same path: still no auto-default because the
	// default already exists (the one we just set via Use). Just sanity-check
	// that the marker file is gone.
	if _, err := os.Stat(filepath.Join(cfg, "last-removed-default-pid")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("marker should be cleared: %v", err)
	}
}
