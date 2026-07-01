package convexport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"/workspace":                           "-workspace",
		"/Users/orest/dev/projects/glovebox":   "-Users-orest-dev-projects-glovebox",
		"/glovebox/84f50c17172b/Users/orest/x": "-glovebox-84f50c17172b-Users-orest-x",
		"/has space/and.dot/and_underscore":    "-has-space-and-dot-and-underscore",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// writeFile creates parent dirs and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExportProject_Claude(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "84f50c17172b"
	ws := "/Users/orest/dev/projects/glovebox"

	projDir := filepath.Join(state, "projects", pid)
	writeFile(t, filepath.Join(projDir, "workspace-path"), ws+"\n")
	// One root-workspace session and one run from a /workspace subdir.
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "aaa.jsonl"), `{"cwd":"/workspace"}`)
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace-frontend", "bbb.jsonl"), `{"cwd":"/workspace/frontend"}`)

	// Default mode symlinks (doCopy=false).
	results, err := ExportProject(state, home, pid, "", "", false)
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}

	// Claude session should land under a glovebox+pid-tagged slug derived from
	// the REAL workspace path, with the subdir suffix preserved.
	wantRoot := "-glovebox-" + pid + "-Users-orest-dev-projects-glovebox"
	wantA := filepath.Join(home, ".claude", "projects", wantRoot, "aaa.jsonl")
	wantB := filepath.Join(home, ".claude", "projects", wantRoot+"-frontend", "bbb.jsonl")
	for _, p := range []string{wantA, wantB} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected exported file %s: %v", p, err)
		}
	}
	// Default is a symlink pointing at the live source file.
	fi, err := os.Lstat(wantA)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected %s to be a symlink (mode=%v err=%v)", wantA, fi.Mode(), err)
	}
	// Readable through the link, byte-for-byte.
	got, _ := os.ReadFile(wantA)
	if string(got) != `{"cwd":"/workspace"}` {
		t.Errorf("content mismatch: %q", got)
	}

	// Claude result reports 2 files exported; the six others report unsupported.
	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Harness] = r
	}
	if c := byName["claude"]; c.Status != StatusExported || c.Files != 2 {
		t.Errorf("claude result = %+v, want exported/2", c)
	}
	if c := byName["codex"]; c.Status != StatusUnsupported {
		t.Errorf("codex result = %+v, want unsupported", c)
	}
}

func TestExportProject_EmptyWorkspacePath(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "deadbeef"
	projDir := filepath.Join(state, "projects", pid)
	writeFile(t, filepath.Join(projDir, "workspace-path"), "")
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "x.jsonl"), "{}")

	if _, err := ExportProject(state, home, pid, "", "", false); err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	// Falls back to a pid-only slug; still unique and obviously glovebox.
	want := filepath.Join(home, ".claude", "projects", "-glovebox-"+pid, "x.jsonl")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s: %v", want, err)
	}
}

func TestExportProject_CopyMode(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "abc123"
	projDir := filepath.Join(state, "projects", pid)
	src := filepath.Join(projDir, "claude", "projects", "-workspace", "s.jsonl")
	writeFile(t, filepath.Join(projDir, "workspace-path"), "/w\n")
	writeFile(t, src, "original")

	if _, err := ExportProject(state, home, pid, "claude", "", true); err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	dst := filepath.Join(home, ".claude", "projects", "-glovebox-"+pid+"-w", "s.jsonl")

	// With --copy the destination is a standalone regular file, not a symlink.
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("stat %s: %v", dst, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("copy mode produced a symlink, want a regular file")
	}
	// It's a snapshot: mutating the source does not change the copy.
	if err := os.WriteFile(src, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "original" {
		t.Errorf("copy tracked the source; got %q, want %q", got, "original")
	}
}

func TestExportProject_ReexportSwitchesMode(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "d00d"
	projDir := filepath.Join(state, "projects", pid)
	writeFile(t, filepath.Join(projDir, "workspace-path"), "/w\n")
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "s.jsonl"), "x")
	dst := filepath.Join(home, ".claude", "projects", "-glovebox-"+pid+"-w", "s.jsonl")

	// Copy first, then re-export as symlink: the prior regular file is replaced.
	if _, err := ExportProject(state, home, pid, "claude", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportProject(state, home, pid, "claude", "", false); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(dst)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("re-export did not replace copy with symlink (mode=%v err=%v)", fi.Mode(), err)
	}
}

func TestRemoveExports(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()

	// pidLink: exported as symlinks (default). pidCopy: exported with --copy.
	pidLink, pidCopy := "1111aaaa2222", "3333bbbb4444"
	for _, pid := range []string{pidLink, pidCopy} {
		writeFile(t, filepath.Join(state, "projects", pid, "workspace-path"), "/w\n")
		writeFile(t, filepath.Join(state, "projects", pid, "claude", "projects", "-workspace", "s.jsonl"), "x")
	}
	if _, err := ExportProject(state, home, pidLink, "claude", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportProject(state, home, pidCopy, "claude", "", true); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(home, ".claude", "projects", "-glovebox-"+pidLink+"-w")
	copyDir := filepath.Join(home, ".claude", "projects", "-glovebox-"+pidCopy+"-w")

	// Removing pidLink's exports drops the symlink and prunes its now-empty dir.
	n, err := RemoveExports(home, pidLink)
	if err != nil {
		t.Fatalf("RemoveExports: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}
	if _, err := os.Lstat(linkDir); !os.IsNotExist(err) {
		t.Errorf("expected pruned dir %s to be gone, err=%v", linkDir, err)
	}

	// A --copy export is a standalone snapshot: RemoveExports leaves it be.
	n, err = RemoveExports(home, pidCopy)
	if err != nil {
		t.Fatalf("RemoveExports(copy): %v", err)
	}
	if n != 0 {
		t.Errorf("removed = %d copies, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(copyDir, "s.jsonl")); err != nil {
		t.Errorf("copy snapshot should survive: %v", err)
	}
}

func TestExportProject_HarnessFilter(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "p1"
	writeFile(t, filepath.Join(state, "projects", pid, "workspace-path"), "/w\n")
	writeFile(t, filepath.Join(state, "projects", pid, "claude", "projects", "-workspace", "a.jsonl"), "{}")

	results, err := ExportProject(state, home, pid, "claude", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Harness != "claude" {
		t.Fatalf("harness filter returned %+v", results)
	}
}
