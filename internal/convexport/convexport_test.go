package convexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"/workspace":                         "-workspace",
		"/Users/orest/dev/projects/glovebox": "-Users-orest-dev-projects-glovebox",
		"/has space/and.dot/and_underscore":  "-has-space-and-dot-and-underscore",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProvenanceRoot(t *testing.T) {
	cases := []struct{ pid, ws, want string }{
		{"030d91f1f47b", "/Users/orest/dev/projects/gwook", "/Users/orest/dev/projects/gbx-030d91f1f47b-gwook"},
		{"84f50c17172b", "/Users/orest/code/myapp", "/Users/orest/code/gbx-84f50c17172b-myapp"},
		{"deadbeef", "", "/gbx-deadbeef"},
	}
	for _, c := range cases {
		if got := provenanceRoot(c.pid, c.ws); got != c.want {
			t.Errorf("provenanceRoot(%q,%q) = %q, want %q", c.pid, c.ws, got, c.want)
		}
	}
}

func TestRewriteCwd(t *testing.T) {
	// Only the cwd field is retargeted; other "/workspace" mentions are kept.
	in := []byte(`{"cwd":"/workspace","text":"edited /workspace/main.go"}`)
	got := string(rewriteCwd(in, "/Users/o/code/gbx-p1-app"))
	want := `{"cwd":"/Users/o/code/gbx-p1-app","text":"edited /workspace/main.go"}`
	if got != want {
		t.Errorf("rewriteCwd = %q, want %q", got, want)
	}
	// Subdir cwd keeps the tail.
	sub := string(rewriteCwd([]byte(`{"cwd":"/workspace/frontend"}`), "/root"))
	if sub != `{"cwd":"/root/frontend"}` {
		t.Errorf("subdir rewrite = %q", sub)
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

	results, err := ExportProject(state, home, pid, "", "")
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}

	// Folder slug is the slug of the provenance root (gbx-<pid>-<name>), with
	// the subdir suffix preserved.
	root := "-Users-orest-dev-projects-gbx-" + pid + "-glovebox"
	wantA := filepath.Join(home, ".claude", "projects", root, "aaa.jsonl")
	wantB := filepath.Join(home, ".claude", "projects", root+"-frontend", "bbb.jsonl")

	// Files are standalone copies (not symlinks), with cwd retargeted so a viewer
	// attributes them to project "gbx_<pid>_glovebox".
	fi, err := os.Lstat(wantA)
	if err != nil {
		t.Fatalf("stat %s: %v", wantA, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("expected a regular file, got a symlink")
	}
	if got, _ := os.ReadFile(wantA); string(got) != `{"cwd":"/Users/orest/dev/projects/gbx-`+pid+`-glovebox"}` {
		t.Errorf("cwd not rewritten in aaa: %s", got)
	}
	if got, _ := os.ReadFile(wantB); string(got) != `{"cwd":"/Users/orest/dev/projects/gbx-`+pid+`-glovebox/frontend"}` {
		t.Errorf("cwd not rewritten in bbb: %s", got)
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

	if _, err := ExportProject(state, home, pid, "", ""); err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	// Falls back to a pid-only root (/gbx-<pid>); still unique and glovebox-tagged.
	want := filepath.Join(home, ".claude", "projects", "-gbx-"+pid, "x.jsonl")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s: %v", want, err)
	}
}

func TestExportProject_ReexportOverwrites(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "abc123"
	src := filepath.Join(state, "projects", pid, "claude", "projects", "-workspace", "s.jsonl")
	writeFile(t, filepath.Join(state, "projects", pid, "workspace-path"), "/w\n")
	writeFile(t, src, `{"cwd":"/workspace","v":1}`)
	dst := filepath.Join(home, ".claude", "projects", "-gbx-"+pid+"-w", "s.jsonl")

	if _, err := ExportProject(state, home, pid, "claude", ""); err != nil {
		t.Fatal(err)
	}
	// Source changes, re-export: the copy is refreshed (not stale).
	writeFile(t, src, `{"cwd":"/workspace","v":2}`)
	if _, err := ExportProject(state, home, pid, "claude", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), `"v":2`) {
		t.Errorf("re-export did not refresh copy: %s", got)
	}
}

func TestExportProject_RemovesStalePriorExports(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "030d91f1f47b"
	ws := "/Users/orest/dev/projects/gwook"
	projDir := filepath.Join(state, "projects", pid)
	writeFile(t, filepath.Join(projDir, "workspace-path"), ws+"\n")
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "sess.jsonl"), `{"cwd":"/workspace"}`)

	projectsHost := filepath.Join(home, ".claude", "projects")
	// A stale export from the old "-glovebox-<pid>-" scheme, and an unrelated
	// native project that must survive.
	stale := filepath.Join(projectsHost, "-glovebox-"+pid+"-Users-orest-dev-projects-gwook")
	native := filepath.Join(projectsHost, "-Users-orest-dev-projects-other")
	writeFile(t, filepath.Join(stale, "sess.jsonl"), "old")
	writeFile(t, filepath.Join(native, "n.jsonl"), "native")

	if _, err := ExportProject(state, home, pid, "claude", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale export %s should be removed (err=%v)", stale, err)
	}
	if _, err := os.Stat(filepath.Join(native, "n.jsonl")); err != nil {
		t.Errorf("native project must be untouched: %v", err)
	}
	cur := filepath.Join(projectsHost, "-Users-orest-dev-projects-gbx-"+pid+"-gwook", "sess.jsonl")
	if _, err := os.Stat(cur); err != nil {
		t.Errorf("current export missing: %v", err)
	}
	// Exactly one export folder for this pid remains.
	dirs, _ := os.ReadDir(projectsHost)
	n := 0
	for _, d := range dirs {
		if strings.Contains(d.Name(), pid) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected 1 export folder for pid, got %d", n)
	}
}

func TestExportProject_ReexportSamePathInPlace(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "abcd1234"
	projDir := filepath.Join(state, "projects", pid)
	writeFile(t, filepath.Join(projDir, "workspace-path"), "/w\n")
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "s.jsonl"), `{"cwd":"/workspace","v":1}`)

	if _, err := ExportProject(state, home, pid, "claude", ""); err != nil {
		t.Fatal(err)
	}
	// Same scheme + workspace: re-export overwrites the SAME folder in place and
	// does not leave a duplicate.
	writeFile(t, filepath.Join(projDir, "claude", "projects", "-workspace", "s.jsonl"), `{"cwd":"/workspace","v":2}`)
	if _, err := ExportProject(state, home, pid, "claude", ""); err != nil {
		t.Fatal(err)
	}
	dirs, _ := os.ReadDir(filepath.Join(home, ".claude", "projects"))
	n := 0
	for _, d := range dirs {
		if strings.Contains(d.Name(), pid) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected 1 export folder after re-export, got %d", n)
	}
	got, _ := os.ReadFile(filepath.Join(home, ".claude", "projects", "-gbx-"+pid+"-w", "s.jsonl"))
	if !strings.Contains(string(got), `"v":2`) {
		t.Errorf("re-export did not refresh in place: %s", got)
	}
}

func TestExportProject_HarnessFilter(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	pid := "p1"
	writeFile(t, filepath.Join(state, "projects", pid, "workspace-path"), "/w\n")
	writeFile(t, filepath.Join(state, "projects", pid, "claude", "projects", "-workspace", "a.jsonl"), "{}")

	results, err := ExportProject(state, home, pid, "claude", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Harness != "claude" {
		t.Fatalf("harness filter returned %+v", results)
	}
}
