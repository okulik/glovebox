package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRules_FromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "allow.txt")
	if err := os.WriteFile(f, []byte("docker.io\nghcr.io\n# a comment\n\nquay.io\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRules(f, 4, 8<<30)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"docker.io", "ghcr.io", "quay.io"} {
		if _, ok := r.AllowedRegistries[want]; !ok {
			t.Errorf("registry %q not loaded", want)
		}
	}
	if _, ok := r.AllowedRegistries["# a comment"]; ok {
		t.Errorf("comment line was not ignored")
	}
}
