package projectid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashIs12LowercaseHex(t *testing.T) {
	pid, err := Hash("/tmp")
	if err != nil {
		t.Fatalf("Hash(/tmp): %v", err)
	}
	if len(pid) != 12 {
		t.Fatalf("pid length: want 12, got %d (%q)", len(pid), pid)
	}
	for _, r := range pid {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("pid not lowercase hex: %q", pid)
		}
	}
}

func TestHashStableAcrossCalls(t *testing.T) {
	a, err := Hash("/tmp")
	if err != nil {
		t.Fatalf("Hash(/tmp) #1: %v", err)
	}
	b, err := Hash("/tmp")
	if err != nil {
		t.Fatalf("Hash(/tmp) #2: %v", err)
	}
	if a != b {
		t.Fatalf("unstable hash: %q vs %q", a, b)
	}
}

func TestHashDiffersByPath(t *testing.T) {
	a, err := Hash("/tmp")
	if err != nil {
		t.Fatalf("Hash(/tmp): %v", err)
	}
	b, err := Hash("/var")
	if err != nil {
		t.Fatalf("Hash(/var): %v", err)
	}
	if a == b {
		t.Fatalf("expected different pids for /tmp vs /var, got %q for both", a)
	}
}

func TestHashCanonicalisesTrailingDot(t *testing.T) {
	a, err := Hash("/tmp")
	if err != nil {
		t.Fatalf("Hash(/tmp): %v", err)
	}
	b, err := Hash("/tmp/.")
	if err != nil {
		t.Fatalf("Hash(/tmp/.): %v", err)
	}
	if a != b {
		t.Fatalf("expected /tmp and /tmp/. to hash the same, got %q vs %q", a, b)
	}
}

func TestHashFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	a, err := Hash(target)
	if err != nil {
		t.Fatalf("Hash(target): %v", err)
	}
	b, err := Hash(link)
	if err != nil {
		t.Fatalf("Hash(link): %v", err)
	}
	if a != b {
		t.Fatalf("symlink should hash same as target: %q vs %q", a, b)
	}
}

func writeProjects(t *testing.T, dir string, pids ...string) {
	t.Helper()
	projects := filepath.Join(dir, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	for _, p := range pids {
		if err := os.MkdirAll(filepath.Join(projects, p), 0o755); err != nil {
			t.Fatalf("mkdir pid %s: %v", p, err)
		}
	}
}

func TestResolveExactMatch(t *testing.T) {
	dir := t.TempDir()
	writeProjects(t, dir, "aaaa1111bbbb", "cccc2222dddd")
	got, err := Resolve(dir, "aaaa1111bbbb")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "aaaa1111bbbb" {
		t.Fatalf("want aaaa1111bbbb, got %q", got)
	}
}

func TestResolveUniquePrefix(t *testing.T) {
	dir := t.TempDir()
	writeProjects(t, dir, "aaaa1111bbbb", "cccc2222dddd")
	got, err := Resolve(dir, "aa")
	if err != nil {
		t.Fatalf("Resolve(aa): %v", err)
	}
	if got != "aaaa1111bbbb" {
		t.Fatalf("want aaaa1111bbbb, got %q", got)
	}
}

func TestResolveNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeProjects(t, dir, "aaaa1111bbbb")
	_, err := Resolve(dir, "zz")
	if err == nil {
		t.Fatal("want error for no match, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "No project matches") {
		t.Fatalf("error must contain 'No project matches', got %q", got)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	dir := t.TempDir()
	writeProjects(t, dir, "aaaa1111bbbb", "aaaa2222cccc")
	_, err := Resolve(dir, "aaaa")
	if err == nil {
		t.Fatal("want error for ambiguous match, got nil")
	}
	got := err.Error()
	if !strings.Contains(got, "Project id is ambiguous") {
		t.Fatalf("error must contain 'Project id is ambiguous', got %q", got)
	}
	if !strings.Contains(got, "aaaa1111bbbb") || !strings.Contains(got, "aaaa2222cccc") {
		t.Fatalf("error must list both candidates, got %q", got)
	}
}

func TestResolveEmptyStateDir(t *testing.T) {
	dir := t.TempDir() // no projects/ subdir at all
	_, err := Resolve(dir, "anything")
	if err == nil {
		t.Fatal("want error when state has no projects dir, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "No project matches") {
		t.Fatalf("error must contain 'No project matches', got %q", got)
	}
}
