package agent_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/agent"
)

func TestUnderBaseAcceptsPathInside(t *testing.T) {
	base := t.TempDir()
	candidate := filepath.Join(base, "sub", "file.txt")
	got, err := agent.UnderBase(base, candidate)
	if err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("returned path should start with base; got %q want prefix %q", got, base)
	}
}

func TestUnderBaseAcceptsPathEqualToBase(t *testing.T) {
	base := t.TempDir()
	if _, err := agent.UnderBase(base, base); err != nil {
		t.Errorf("base == candidate should be ok: %v", err)
	}
}

func TestUnderBaseRejectsParentEscape(t *testing.T) {
	base := t.TempDir()
	// base/../sneaky resolves to a sibling of base - outside.
	candidate := filepath.Join(base, "..", "sneaky")
	if _, err := agent.UnderBase(base, candidate); err == nil {
		t.Errorf("expected refusal for parent escape")
	}
}

func TestUnderBaseRejectsBoundaryPrefixMatch(t *testing.T) {
	// "/foo/bar" must NOT be considered inside "/foo/ba".
	if _, err := agent.UnderBase("/foo/ba", "/foo/bar/file"); err == nil {
		t.Errorf("expected refusal: /foo/bar/file is not inside /foo/ba")
	}
}

func TestUnderBaseHandlesRelativeInputs(t *testing.T) {
	// Both inputs relative; both should resolve consistently against CWD.
	got, err := agent.UnderBase(".", "./inside/x")
	if err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("result should be absolute, got %q", got)
	}
}

func TestUnderBaseTrailingSlashIsBoundaryAgnostic(t *testing.T) {
	base := t.TempDir()
	candidate := filepath.Join(base, "x")
	if _, err := agent.UnderBase(base+string(filepath.Separator), candidate); err != nil {
		t.Errorf("trailing separator on base should be accepted: %v", err)
	}
}

// TestWriteAtomicReplace verifies that writing over a path with existing
// content replaces it with the new bytes.
func TestWriteAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	want := []byte("new content")
	if err := agent.WriteAtomic(path, want, 0o600); err != nil {
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
		if err := agent.WriteAtomic(path, []byte("x"), perm); err != nil {
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

	if err := agent.WriteAtomic(path, []byte("x"), 0o600); err == nil {
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

	if err := agent.WriteAtomic(path, []byte("x"), 0o600); err == nil {
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

func TestDeepMergeJSON_UnionsAllowList(t *testing.T) {
	dst := map[string]any{"permissions": map[string]any{"allow": []any{"Read(//data/**)"}}}
	src := map[string]any{"permissions": map[string]any{"allow": []any{"Bash(gbx-stack *)", "Read(//data/**)"}}}
	got := agent.DeepMergeJSON(dst, src).(map[string]any)
	allow := got["permissions"].(map[string]any)["allow"].([]any)
	// user entry kept first, new entry appended, exact dup not duplicated.
	if len(allow) != 2 {
		t.Fatalf("allow = %v, want 2 entries", allow)
	}
	if allow[0] != "Read(//data/**)" || allow[1] != "Bash(gbx-stack *)" {
		t.Errorf("allow order/content = %v", allow)
	}
}

func TestDeepMergeJSON_ScalarUserWinsFillsMissing(t *testing.T) {
	dst := map[string]any{"model": "opus"}
	src := map[string]any{"model": "sonnet", "theme": "dark"}
	got := agent.DeepMergeJSON(dst, src).(map[string]any)
	if got["model"] != "opus" {
		t.Errorf("model = %v, user value must win", got["model"])
	}
	if got["theme"] != "dark" {
		t.Errorf("theme = %v, missing key must be filled", got["theme"])
	}
}

func TestDeepMergeJSON_Idempotent(t *testing.T) {
	dst := map[string]any{"a": float64(1), "list": []any{"x"}}
	src := map[string]any{"a": float64(2), "list": []any{"x", "y"}, "b": "new"}
	once := agent.DeepMergeJSON(dst, src)
	twice := agent.DeepMergeJSON(once, src)
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("not idempotent:\n once=%v\n twice=%v", once, twice)
	}
}
