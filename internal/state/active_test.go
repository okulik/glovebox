package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivePIDMissing(t *testing.T) {
	dir := t.TempDir()
	pid, err := ActivePID(dir)
	if err != nil {
		t.Fatalf("ActivePID on missing file: %v", err)
	}
	if pid != "" {
		t.Fatalf("want empty pid for missing file, got %q", pid)
	}
}

func TestActivePathMissing(t *testing.T) {
	dir := t.TempDir()
	ws, err := ActivePath(dir)
	if err != nil {
		t.Fatalf("ActivePath on missing file: %v", err)
	}
	if ws != "" {
		t.Fatalf("want empty path for missing file, got %q", ws)
	}
}

func TestActivePIDPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "active-project"),
		[]byte("aaaa1111bbbb\n/work/foo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	pid, err := ActivePID(dir)
	if err != nil {
		t.Fatalf("ActivePID: %v", err)
	}
	if pid != "aaaa1111bbbb" {
		t.Fatalf("want aaaa1111bbbb, got %q", pid)
	}
}

func TestActivePathPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "active-project"),
		[]byte("aaaa1111bbbb\n/work/foo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	ws, err := ActivePath(dir)
	if err != nil {
		t.Fatalf("ActivePath: %v", err)
	}
	if ws != "/work/foo" {
		t.Fatalf("want /work/foo, got %q", ws)
	}
}

func TestActivePIDOneLineFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "active-project"),
		[]byte("aaaa1111bbbb\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	pid, err := ActivePID(dir)
	if err != nil {
		t.Fatalf("ActivePID: %v", err)
	}
	if pid != "aaaa1111bbbb" {
		t.Fatalf("want aaaa1111bbbb, got %q", pid)
	}
	ws, err := ActivePath(dir)
	if err != nil {
		t.Fatalf("ActivePath: %v", err)
	}
	if ws != "" {
		t.Fatalf("want empty path, got %q", ws)
	}
}

func TestWriteActiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteActive(dir, "aaaa1111bbbb", "/work/foo"); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}
	pid, err := ActivePID(dir)
	if err != nil || pid != "aaaa1111bbbb" {
		t.Fatalf("read-back pid: got %q err=%v", pid, err)
	}
	ws, err := ActivePath(dir)
	if err != nil || ws != "/work/foo" {
		t.Fatalf("read-back path: got %q err=%v", ws, err)
	}
}

func TestWriteActiveCreatesConfigDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "config")
	if err := WriteActive(nested, "aaaa1111bbbb", "/work/foo"); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, "active-project")); err != nil {
		t.Fatalf("file not created in nested dir: %v", err)
	}
}

func TestWriteActiveAtomicNoTmpfileLeak(t *testing.T) {
	dir := t.TempDir()
	if err := WriteActive(dir, "aaaa1111bbbb", "/work/foo"); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "active-project" {
			t.Fatalf("tmpfile leak: %q", e.Name())
		}
	}
}

func TestWriteActiveOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := WriteActive(dir, "aaaa1111bbbb", "/work/foo"); err != nil {
		t.Fatalf("first WriteActive: %v", err)
	}
	if err := WriteActive(dir, "cccc2222dddd", "/work/bar"); err != nil {
		t.Fatalf("second WriteActive: %v", err)
	}
	pid, _ := ActivePID(dir)
	ws, _ := ActivePath(dir)
	if pid != "cccc2222dddd" || ws != "/work/bar" {
		t.Fatalf("want overwrite to cccc/bar, got %q/%q", pid, ws)
	}
}
