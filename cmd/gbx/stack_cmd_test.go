package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStackServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestStackLs(t *testing.T) {
	srv := newStackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`["pid1","pid2"]`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	stdout, _, code := runCLI(t, "stack", "ls")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "pid1") || !strings.Contains(stdout, "pid2") {
		t.Fatalf("stdout: %q", stdout)
	}
}

func TestStackStatusUsesProjectID(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"ready"}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	stdout, _, code := runCLI(t, "stack", "status")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "ready") {
		t.Fatalf("stdout: %q", stdout)
	}
	if sawPath != "/projects/myproj/status" {
		t.Errorf("path: %q", sawPath)
	}
}

func TestStackStatusOverridePIDWins(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	// GBX_OVERRIDE_PID is what the global -p/--pid flag resolves to; it must
	// win over GBX_PROJECT_ID.
	t.Setenv("GBX_PROJECT_ID", "envproj")
	t.Setenv("GBX_OVERRIDE_PID", "flagproj")
	_, _, code := runCLI(t, "stack", "status")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/flagproj/status" {
		t.Errorf("-p/--pid (GBX_OVERRIDE_PID) must override GBX_PROJECT_ID: %q", sawPath)
	}
}

func TestStackDown(t *testing.T) {
	var sawMethod, sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	_, _, code := runCLI(t, "stack", "down")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawMethod != "POST" || sawPath != "/projects/myproj/down" {
		t.Errorf("got %s %s", sawMethod, sawPath)
	}
}

func TestStackDestroyYes(t *testing.T) {
	var sawPath, sawQuery string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "p1")
	_, _, code := runCLI(t, "stack", "destroy", "-y")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/p1/destroy" {
		t.Errorf("path: %q", sawPath)
	}
	if sawQuery != "confirm=true" {
		t.Errorf("query: %q", sawQuery)
	}
}

// withStdin replaces os.Stdin with a pipe carrying s for the duration of the
// test, so commands that prompt via confirmYN read a scripted answer.
func withStdin(t *testing.T, s string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(s); err != nil {
		t.Fatal(err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})
}

func TestStackDestroyPromptProceedsOnYes(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "p1")
	withStdin(t, "y\n")
	_, _, code := runCLI(t, "stack", "destroy")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/p1/destroy" {
		t.Errorf("path: %q", sawPath)
	}
}

func TestStackDestroyPromptAbortsOnNo(t *testing.T) {
	srv := newStackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("destroy must not contact the controller when the prompt is declined")
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "p1")
	withStdin(t, "n\n")
	_, _, code := runCLI(t, "stack", "destroy")
	if code == 0 {
		t.Fatal("expected non-zero exit when prompt declined")
	}
}

func TestStackDestroyOverridePIDWins(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	// GBX_OVERRIDE_PID (set by the global -p/--pid flag) must win over
	// GBX_PROJECT_ID.
	t.Setenv("GBX_PROJECT_ID", "envproj")
	t.Setenv("GBX_OVERRIDE_PID", "flagproj")
	_, _, code := runCLI(t, "stack", "destroy", "-y")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/flagproj/destroy" {
		t.Errorf("-p/--pid (GBX_OVERRIDE_PID) must override GBX_PROJECT_ID: %q", sawPath)
	}
}

func TestStackApplyPromptProceedsOnYes(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"applied"}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	withStdin(t, "y\n")
	stdout, _, code := runCLI(t, "stack", "apply")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/myproj/apply" {
		t.Errorf("path: %q", sawPath)
	}
	if !strings.Contains(stdout, "applied") {
		t.Errorf("stdout: %q", stdout)
	}
}

func TestStackLogs(t *testing.T) {
	srv := newStackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("log line 1\nlog line 2\n"))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "p1")
	stdout, _, code := runCLI(t, "stack", "logs", "redis")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "log line 1") {
		t.Errorf("stdout: %q", stdout)
	}
}

func TestStackImageAllowAppendsIfMissing(t *testing.T) {
	libexec := t.TempDir()
	dockerDir := filepath.Join(libexec, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	allowPath := filepath.Join(dockerDir, "image-allowlist.txt")
	if err := os.WriteFile(allowPath, []byte("registry.io\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GBX_LIBEXEC", libexec)
	_, _, code := runCLI(t, "stack", "image-allow", "new-registry.io")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	data, _ := os.ReadFile(allowPath)
	if !strings.Contains(string(data), "new-registry.io") {
		t.Errorf("allowlist did not get the new entry: %q", data)
	}
}

func TestStackImageAllowSkipsIfAlreadyPresent(t *testing.T) {
	libexec := t.TempDir()
	dockerDir := filepath.Join(libexec, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	allowPath := filepath.Join(dockerDir, "image-allowlist.txt")
	if err := os.WriteFile(allowPath, []byte("registry.io\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GBX_LIBEXEC", libexec)
	stdout, _, code := runCLI(t, "stack", "image-allow", "registry.io")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "already") {
		t.Errorf("stdout should mention already-allowed: %q", stdout)
	}
	data, _ := os.ReadFile(allowPath)
	if strings.Count(string(data), "registry.io") != 1 {
		t.Errorf("registry.io duplicated: %q", data)
	}
}

func TestStackDiffRendersManifests(t *testing.T) {
	var sawPath string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"live":"version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n","proposed":"version: 1\nservices:\n  redis:\n    image: redis:8\n"}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	stdout, _, code := runCLI(t, "stack", "diff")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/myproj/manifests" {
		t.Errorf("path = %q", sawPath)
	}
	if !strings.Contains(stdout, "-    image: redis:7-alpine") || !strings.Contains(stdout, "+    image: redis:8") {
		t.Fatalf("diff stdout: %q", stdout)
	}
}

func TestStackDiffNoProposal(t *testing.T) {
	srv := newStackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"live":"version: 1\n","proposed":null}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	stdout, _, code := runCLI(t, "stack", "diff")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout, "no pending proposal") {
		t.Fatalf("stdout: %q", stdout)
	}
}

func TestStackApplyPostsEmptyBody(t *testing.T) {
	var sawPath, sawBody string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		sawBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"applied"}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	stdout, _, code := runCLI(t, "stack", "apply", "-y")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if sawPath != "/projects/myproj/apply" {
		t.Errorf("path = %q", sawPath)
	}
	if sawBody != "" {
		t.Errorf("expected empty body (apply stored proposal), got %q", sawBody)
	}
	if !strings.Contains(stdout, "applied") {
		t.Errorf("stdout: %q", stdout)
	}
}

func TestStackApplyRequiresYes(t *testing.T) {
	srv := newStackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("apply without -y must not contact the controller")
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	_, _, code := runCLI(t, "stack", "apply")
	if code == 0 {
		t.Fatal("expected non-zero exit without -y")
	}
}

func TestStackApplyDryRunShowsProposal(t *testing.T) {
	var postPaths []string
	srv := newStackServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postPaths = append(postPaths, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"live":null,"proposed":"version: 1\nservices:\n  redis:\n    image: redis:8\n"}`))
	})
	defer srv.Close()
	t.Setenv("GBX_CONTROLLER_URL", srv.URL)
	t.Setenv("GBX_PROJECT_ID", "myproj")
	stdout, _, code := runCLI(t, "stack", "apply", "--dry-run")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if len(postPaths) != 0 {
		t.Errorf("dry-run must not POST, got %v", postPaths)
	}
	if !strings.Contains(stdout, "image: redis:8") {
		t.Errorf("expected proposed manifest in stdout: %q", stdout)
	}
}
