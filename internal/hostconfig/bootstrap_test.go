package hostconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubLibexec creates a fake repo libexec tree that Bootstrap will copy from.
func stubLibexec(t *testing.T) string {
	t.Helper()
	libexec := t.TempDir()
	if err := os.WriteFile(filepath.Join(libexec, ".env.example"),
		[]byte("FOO=1\nBAR=2\n"), 0o644); err != nil {
		t.Fatalf("seed env.example: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(libexec, "docker", "proxy"), 0o755); err != nil {
		t.Fatalf("mkdir docker/proxy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libexec, "docker", "proxy", "allowlist.txt"),
		[]byte("example.com\n"), 0o644); err != nil {
		t.Fatalf("seed allowlist: %v", err)
	}
	return libexec
}

func TestBootstrapCreatesStateSkeleton(t *testing.T) {
	libexec := stubLibexec(t)
	config := t.TempDir()
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	for _, dir := range []string{
		"state/projects",
		"state/shared/npm",
		"state/shared/uv-tools",
		"state/shared/bin",
		"state/shared/cache",
		"state/shared/shell-history",
	} {
		if _, err := os.Stat(filepath.Join(config, dir)); err != nil {
			t.Errorf("missing %s: %v", dir, err)
		}
	}
}

func TestBootstrapSeedsFiles(t *testing.T) {
	libexec := stubLibexec(t)
	config := t.TempDir()
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	for _, f := range []string{".env", "allowlist.txt"} {
		if _, err := os.Stat(filepath.Join(config, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestBootstrapIsIdempotent(t *testing.T) {
	libexec := stubLibexec(t)
	config := t.TempDir()
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	envPath := filepath.Join(config, ".env")
	if err := os.WriteFile(envPath, []byte("FOO=user-edit\nBAR=2\n"), 0o644); err != nil {
		t.Fatalf("re-write env: %v", err)
	}
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	data, _ := os.ReadFile(envPath)
	if !strings.Contains(string(data), "user-edit") {
		t.Fatalf("user-edited .env was overwritten: %q", data)
	}
}

func TestBootstrapSyncEnvAppendsMissingKeys(t *testing.T) {
	libexec := stubLibexec(t)
	config := t.TempDir()
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	envPath := filepath.Join(config, ".env")
	if err := os.WriteFile(envPath, []byte("FOO=1\n"), 0o644); err != nil {
		t.Fatalf("re-write env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libexec, ".env.example"),
		[]byte("FOO=1\nBAR=2\nBAZ=3\n"), 0o644); err != nil {
		t.Fatalf("rewrite template: %v", err)
	}
	if err := Bootstrap(libexec, config); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	data, _ := os.ReadFile(envPath)
	for _, key := range []string{"BAR=", "BAZ="} {
		if !strings.Contains(string(data), key) {
			t.Errorf("expected %q to be appended, env now: %q", key, data)
		}
	}
}
