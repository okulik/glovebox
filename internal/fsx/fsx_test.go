package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okulik/glovebox/internal/fsx"
)

// TestWriteAtomicReplace verifies that writing over a path with existing
// content replaces it with the new bytes.
func TestWriteAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	want := []byte("new content")
	if err := fsx.WriteAtomic(path, want, 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

// TestWriteAtomicPermissions verifies the on-disk mode equals the requested
// perm exactly, regardless of umask (we Chmod explicitly).
func TestWriteAtomicPermissions(t *testing.T) {
	for _, perm := range []os.FileMode{0o600, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.txt")
		if err := fsx.WriteAtomic(path, []byte("x"), perm); err != nil {
			t.Fatalf("WriteAtomic perm %o: %v", perm, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := info.Mode().Perm(); got != perm {
			t.Fatalf("perm = %o, want %o", got, perm)
		}
	}
}

// TestWriteAtomicTempCleanupOnCreateError forces CreateTemp to fail (parent dir
// does not exist) and asserts an error is returned and no temp files leak.
func TestWriteAtomicTempCleanupOnCreateError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	path := filepath.Join(missing, "file.txt")

	if err := fsx.WriteAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("WriteAtomic returned nil error, want failure")
	}

	// The missing parent dir must not have been created, and dir itself must
	// hold no leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("leftover entries in dir: %v", names)
	}
}

// TestWriteAtomicTempCleanupOnRenameError forces Rename to fail by making the
// destination an existing directory, then asserts no temp file leaks beside it.
func TestWriteAtomicTempCleanupOnRenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dest")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	if err := fsx.WriteAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("WriteAtomic returned nil error, want failure")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	// Only the "dest" directory should remain; no .tmp leftovers.
	for _, e := range entries {
		if e.Name() != "dest" {
			t.Fatalf("leftover temp entry: %s", e.Name())
		}
	}
}
