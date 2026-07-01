package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/agent"
)

// agentTable's key set must equal the canonical agent.Names, so the in-container
// dispatch allowlist can't drift from the rest of the codebase.
func TestAgentTableMatchesNames(t *testing.T) {
	got := make([]string, 0, len(agentTable))
	for k := range agentTable {
		got = append(got, k)
	}
	want := append([]string(nil), agent.Names...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("agentTable keys = %v, want %v", got, want)
	}
}

// fakeRunner records exec calls and simulates install/exec results.
type fakeRunner struct {
	installErr   error
	execErr      error
	statErr      map[string]error
	installCalls []string
	execCalls    []execRecord
}

type execRecord struct {
	argv0  string
	envHas string
	argv   []string
}

func (f *fakeRunner) stat(path string) (os.FileInfo, error) {
	if err := f.statErr[path]; err != nil {
		return nil, err
	}
	return nil, nil
}

func (f *fakeRunner) install(name string) error {
	f.installCalls = append(f.installCalls, name)
	return f.installErr
}

func (f *fakeRunner) exec(argv0 string, argv []string, env []string) error {
	rec := execRecord{argv0: argv0, argv: argv}
	for _, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			rec.envHas = "HOME"
			break
		}
	}
	f.execCalls = append(f.execCalls, rec)
	return f.execErr
}

func TestDispatchUnknownAgentErrors(t *testing.T) {
	r := &fakeRunner{}
	err := dispatchWith(r, "/usr/local/bin/nosuch", []string{"--help"}, "/workspace")
	if err == nil {
		t.Fatal("want error for unknown agent name")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error must mention 'unknown', got %q", err)
	}
}

func TestDispatchKnownAgentExecsRealBinary(t *testing.T) {
	r := &fakeRunner{
		statErr: map[string]error{
			"/home/gbx/.npm/bin/claude": nil,
		},
	}
	err := dispatchWith(r, "/usr/local/bin/claude", []string{"--version"}, "/workspace/proj")
	if err != nil {
		t.Fatalf("dispatchWith: %v", err)
	}
	if len(r.execCalls) != 1 {
		t.Fatalf("want 1 exec call, got %d", len(r.execCalls))
	}
	if r.execCalls[0].argv0 != "/home/gbx/.npm/bin/claude" {
		t.Errorf("argv0: %q", r.execCalls[0].argv0)
	}
	if len(r.execCalls[0].argv) < 1 || r.execCalls[0].argv[0] != "claude" {
		t.Errorf("argv[0] should be the basename: %v", r.execCalls[0].argv)
	}
	if len(r.execCalls[0].argv) < 2 || r.execCalls[0].argv[1] != "--version" {
		t.Errorf("argv[1] should be the passed flag: %v", r.execCalls[0].argv)
	}
	if len(r.installCalls) != 0 {
		t.Errorf("install must NOT be called when binary exists: %v", r.installCalls)
	}
}

func TestDispatchInstallsWhenBinaryMissing(t *testing.T) {
	r := &fakeRunner{
		statErr: map[string]error{
			"/home/gbx/.npm/bin/claude": os.ErrNotExist,
		},
	}
	err := dispatchWith(r, "/usr/local/bin/claude", []string{}, "/workspace/proj")
	if err != nil {
		t.Fatalf("dispatchWith: %v", err)
	}
	if len(r.installCalls) != 1 || r.installCalls[0] != "claude" {
		t.Errorf("install must be called once for claude: %v", r.installCalls)
	}
}

func TestDispatchPropagatesInstallError(t *testing.T) {
	r := &fakeRunner{
		statErr:    map[string]error{"/home/gbx/.npm/bin/claude": os.ErrNotExist},
		installErr: errors.New("npm exploded"),
	}
	err := dispatchWith(r, "/usr/local/bin/claude", []string{}, "/workspace")
	if err == nil {
		t.Fatal("want install error")
	}
	if !strings.Contains(err.Error(), "npm exploded") {
		t.Errorf("error: %v", err)
	}
}

func TestDispatchUVInstalledAgentUsesUVPath(t *testing.T) {
	r := &fakeRunner{
		statErr: map[string]error{"/home/gbx/.local/bin/aider": nil},
	}
	err := dispatchWith(r, "/usr/local/bin/aider", []string{}, "/workspace")
	if err != nil {
		t.Fatalf("dispatchWith: %v", err)
	}
	if r.execCalls[0].argv0 != "/home/gbx/.local/bin/aider" {
		t.Errorf("aider should exec from UV bin dir, got %q", r.execCalls[0].argv0)
	}
}

func TestResolveAgent(t *testing.T) {
	cases := map[string]struct {
		path string
		err  bool
	}{
		"claude":   {"/home/gbx/.npm/bin/claude", false},
		"codex":    {"/home/gbx/.npm/bin/codex", false},
		"gemini":   {"/home/gbx/.npm/bin/gemini", false},
		"opencode": {"/home/gbx/.npm/bin/opencode", false},
		"pi":       {"/home/gbx/.npm/bin/pi", false},
		"aider":    {"/home/gbx/.local/bin/aider", false},
		"hermes":   {"/home/gbx/.local/bin/hermes", false},
		"nosuch":   {"", true},
	}
	for name, want := range cases {
		path, err := resolveAgent(name)
		if want.err {
			if err == nil {
				t.Errorf("%s: want error, got path %q", name, path)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
		if path != want.path {
			t.Errorf("%s: want path %q, got %q", name, want.path, path)
		}
	}
}

func TestDispatchChdirsToWorkspace(t *testing.T) {
	if got := shouldChdir("/tmp"); !got {
		t.Error("shouldChdir(/tmp) should be true")
	}
	if got := shouldChdir("/workspace"); got {
		t.Error("shouldChdir(/workspace) should be false")
	}
	if got := shouldChdir("/workspace/sub/dir"); got {
		t.Error("shouldChdir(/workspace/sub/dir) should be false")
	}
}

func TestAgentNameExtraction(t *testing.T) {
	if got := filepath.Base("/usr/local/bin/claude"); got != "claude" {
		t.Errorf("got %q", got)
	}
}
