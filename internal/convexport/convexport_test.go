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
