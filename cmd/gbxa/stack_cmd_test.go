package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/config"
)

func TestDefaultToHelp(t *testing.T) {
	if got := defaultToHelp(nil); len(got) != 1 || got[0] != "--help" {
		t.Errorf("empty argv should map to [--help], got %v", got)
	}
	if got := defaultToHelp([]string{"status"}); len(got) != 1 || got[0] != "status" {
		t.Errorf("non-empty argv must pass through unchanged, got %v", got)
	}
}

func TestStackBareInvocationListsCommands(t *testing.T) {
	var cli AgentStackCmd
	var out bytes.Buffer
	parser, err := kong.New(&cli, kong.Name("gbx-stack"),
		kong.ConfigureHelp(kong.HelpOptions{WrapUpperBound: helpWrapWidth}),
		kong.Writers(&out, &out),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	// defaultToHelp(nil) turns a bare `gbx-stack` into a help request. Kong's
	// help flag prints usage and then calls Exit(0); in production that
	// terminates the process, but with a no-op Exit here parsing continues and
	// reports the missing command, so we ignore the parse error and assert on
	// the help text that was already written.
	_, _ = parser.Parse(defaultToHelp(nil))
	got := out.String()
	if !strings.Contains(got, "Usage: gbx-stack") {
		t.Fatalf("bare invocation should print usage: %q", got)
	}
	for _, sub := range []string{"status", "diff", "start", "stop", "reset", "propose", "wait", "logs", "info"} {
		if !strings.Contains(got, sub) {
			t.Errorf("help missing subcommand %q: %q", sub, got)
		}
	}
}

func TestAgentProposePostsManifest(t *testing.T) {
	var sawPath, sawBody, sawCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		sawBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"proposed"}`))
	}))
	defer srv.Close()
	t.Setenv(config.EnvControllerURL, srv.URL)
	t.Setenv(config.EnvProjectID, "myproj")

	dir := t.TempDir()
	src := filepath.Join(dir, "candidate.yml")
	content := "version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n"
	if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &AgentStackProposeCmd{Source: src}
	old := os.Stdout
	rPipe, wPipe, _ := os.Pipe()
	os.Stdout = wPipe
	runErr := cmd.Run()
	_ = wPipe.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rPipe)
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if !strings.Contains(buf.String(), `"status":"proposed"`) {
		t.Errorf("output missing controller response: %s", buf.String())
	}
	if sawPath != "/projects/myproj/propose" {
		t.Errorf("path = %q", sawPath)
	}
	if sawBody != content {
		t.Errorf("body = %q", sawBody)
	}
	if sawCT != "text/yaml" {
		t.Errorf("content-type = %q", sawCT)
	}
}

func TestAgentDiffReadsManifests(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"live":"version: 1\n","proposed":"version: 1\nservices:\n  redis:\n    image: redis:8\n"}`))
	}))
	defer srv.Close()
	t.Setenv(config.EnvControllerURL, srv.URL)
	t.Setenv(config.EnvProjectID, "myproj")

	cmd := &AgentStackDiffCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if sawPath != "/projects/myproj/manifests" {
		t.Errorf("path = %q", sawPath)
	}
}
