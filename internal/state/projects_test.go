package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireSomeProjectErrorWhenDirMissing(t *testing.T) {
	dir := t.TempDir()
	err := RequireSomeProject(dir)
	if err == nil {
		t.Fatal("want error when projects dir is missing, got nil")
	}
	if !strings.Contains(err.Error(), "No projects.") {
		t.Fatalf("error must contain 'No projects.', got %q", err)
	}
	if !strings.Contains(err.Error(), "gbx new") {
		t.Fatalf("error must mention 'gbx new', got %q", err)
	}
}

func TestRequireSomeProjectErrorWhenDirEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := RequireSomeProject(dir)
	if err == nil {
		t.Fatal("want error when projects dir is empty, got nil")
	}
	if !strings.Contains(err.Error(), "No projects.") {
		t.Fatalf("error must contain 'No projects.', got %q", err)
	}
}

func TestRequireSomeProjectOKWhenPopulated(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects", "aaaa1111bbbb"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := RequireSomeProject(dir); err != nil {
		t.Fatalf("RequireSomeProject: want nil, got %v", err)
	}
}
