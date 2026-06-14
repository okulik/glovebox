package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordingExec captures the commands run by Install/Update so tests can
// assert exact argv.
type recordingExec struct {
	calls [][]string
	err   error
	out   []byte
}

func (r *recordingExec) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.out, r.err
}

func joined(c []string) string { return strings.Join(c, " ") }

func TestInstallClaude(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "claude"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(r.calls) < 1 {
		t.Fatal("expected at least one call")
	}
	if joined(r.calls[0]) != "npm install -g @anthropic-ai/claude-code" {
		t.Fatalf("unexpected primary call: %q", joined(r.calls[0]))
	}
}

func TestInstallCodex(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "codex"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "npm install -g @openai/codex" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestInstallOpencode(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "opencode"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "npm install -g opencode-ai" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestInstallPi(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "pi"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "npm install -g @earendil-works/pi-coding-agent" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestInstallGemini(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "gemini"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "npm install -g @google/gemini-cli" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestInstallAider(t *testing.T) {
	r := &recordingExec{}
	if err := Install(context.Background(), r, "aider"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "uv tool install aider-chat" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestInstallHermesUsesResolvedTag(t *testing.T) {
	r := &recordingExec{}
	origResolver := hermesTagResolver
	defer func() { hermesTagResolver = origResolver }()
	hermesTagResolver = func(context.Context) (string, error) { return "v1.2.3", nil }

	if err := Install(context.Background(), r, "hermes"); err != nil {
		t.Fatalf("Install hermes: %v", err)
	}
	want := "uv tool install git+https://github.com/NousResearch/hermes-agent@v1.2.3"
	if joined(r.calls[0]) != want {
		t.Fatalf("got %q, want %q", joined(r.calls[0]), want)
	}
}

func TestInstallHermesPropagatesResolverError(t *testing.T) {
	r := &recordingExec{}
	origResolver := hermesTagResolver
	defer func() { hermesTagResolver = origResolver }()
	hermesTagResolver = func(context.Context) (string, error) { return "", errors.New("offline") }

	err := Install(context.Background(), r, "hermes")
	if err == nil {
		t.Fatal("expected resolver error to propagate")
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Fatalf("error must wrap resolver err: %v", err)
	}
}

func TestInstallUnknownAgent(t *testing.T) {
	err := Install(context.Background(), &recordingExec{}, "nosuch")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("error must mention 'unknown agent': %v", err)
	}
}

func TestInstallPropagatesExecutorError(t *testing.T) {
	r := &recordingExec{err: errors.New("npm exploded")}
	err := Install(context.Background(), r, "claude")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "npm exploded") {
		t.Fatalf("error: %v", err)
	}
}

func TestUpdateClaude(t *testing.T) {
	r := &recordingExec{}
	if err := Update(context.Background(), r, "claude"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "npm install -g @anthropic-ai/claude-code@latest" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestUpdateAider(t *testing.T) {
	r := &recordingExec{}
	if err := Update(context.Background(), r, "aider"); err != nil {
		t.Fatal(err)
	}
	if joined(r.calls[0]) != "uv tool upgrade aider-chat" {
		t.Fatalf("got: %q", joined(r.calls[0]))
	}
}

func TestUpdateHermesUsesResolvedTag(t *testing.T) {
	r := &recordingExec{}
	origResolver := hermesTagResolver
	defer func() { hermesTagResolver = origResolver }()
	hermesTagResolver = func(context.Context) (string, error) { return "v9.9.9", nil }

	if err := Update(context.Background(), r, "hermes"); err != nil {
		t.Fatal(err)
	}
	want := "uv tool install --reinstall git+https://github.com/NousResearch/hermes-agent@v9.9.9"
	if joined(r.calls[0]) != want {
		t.Fatalf("got %q, want %q", joined(r.calls[0]), want)
	}
}

func TestUpdateUnknownAgent(t *testing.T) {
	err := Update(context.Background(), &recordingExec{}, "nosuch")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestInstallArgvForKnownAgent(t *testing.T) {
	argv, err := InstallArgv(context.Background(), "claude")
	if err != nil {
		t.Fatalf("InstallArgv: %v", err)
	}
	if joined(argv) != "npm install -g @anthropic-ai/claude-code" {
		t.Fatalf("argv = %q", joined(argv))
	}
}

func TestUpdateArgvForKnownAgent(t *testing.T) {
	argv, err := UpdateArgv(context.Background(), "aider")
	if err != nil {
		t.Fatalf("UpdateArgv: %v", err)
	}
	if joined(argv) != "uv tool upgrade aider-chat" {
		t.Fatalf("argv = %q", joined(argv))
	}
}

func TestUpdateArgvHermesPropagatesResolverError(t *testing.T) {
	origResolver := hermesTagResolver
	defer func() { hermesTagResolver = origResolver }()
	hermesTagResolver = func(context.Context) (string, error) { return "", errors.New("offline") }

	_, err := UpdateArgv(context.Background(), "hermes")
	if err == nil {
		t.Fatal("want error from resolver")
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Fatalf("error: %v", err)
	}
}

func TestInstallArgvUnknownAgent(t *testing.T) {
	_, err := InstallArgv(context.Background(), "nosuch")
	if err == nil {
		t.Fatal("want error for unknown agent")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("error: %v", err)
	}
}
