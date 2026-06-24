package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/agent"
)

// instructionsTestEnv returns the (stateProjDir, dockerDir) pair the helpers
// expect, with `defaults/agent-instructions.md` written next to
// dockerDir. An empty `source` skips writing the file (to exercise the
// "source missing -> no-op" branch).
func instructionsTestEnv(t *testing.T, source string) (string, string) {
	t.Helper()
	root := t.TempDir()
	stateProjDir := filepath.Join(root, "state")
	dockerDir := filepath.Join(root, "libexec", "docker")
	defaultsDir := filepath.Join(root, "libexec", "defaults")
	for _, d := range []string{stateProjDir, dockerDir, defaultsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if source != "" {
		if err := os.WriteFile(filepath.Join(defaultsDir, "agent-instructions.md"),
			[]byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return stateProjDir, dockerDir
}

func TestInjectAgentInstructionsCreatesAllThreeFiles(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "# Guidance\n\nDo X.\n")
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatalf("inject: %v", err)
	}
	for _, rel := range agent.AgentInstructionTargets {
		full := filepath.Join(stateProj, rel)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("expected %s: %v", full, err)
		}
		s := string(data)
		if !strings.Contains(s, agent.InstructionsMarkerBegin) {
			t.Errorf("%s missing begin marker", rel)
		}
		if !strings.Contains(s, agent.InstructionsMarkerEnd) {
			t.Errorf("%s missing end marker", rel)
		}
		if !strings.Contains(s, "Do X.") {
			t.Errorf("%s missing canonical content", rel)
		}
	}
}

func TestInjectAgentInstructionsIsIdempotent(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "# Guidance\nrules\n")
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateProj, "claude/CLAUDE.md")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infoFirst, _ := os.Stat(path)

	// Re-run.
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	infoSecond, _ := os.Stat(path)

	if string(first) != string(second) {
		t.Errorf("content changed across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if !infoFirst.ModTime().Equal(infoSecond.ModTime()) {
		t.Errorf("mtime changed across no-op re-run (rewrite should be skipped when bytes match)")
	}
}

func TestInjectAgentInstructionsAppendsToExistingUnmarkedFile(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "rules text\n")
	existing := "# My project notes\n\nThis is mine.\n"
	path := filepath.Join(stateProj, "claude/CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.HasPrefix(s, existing) {
		t.Errorf("existing prefix not preserved:\n%s", s)
	}
	if !strings.Contains(s, agent.InstructionsMarkerBegin) || !strings.Contains(s, "rules text") {
		t.Errorf("block not appended:\n%s", s)
	}
}

func TestInjectAgentInstructionsReplacesMarkedBlock(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "NEW CONTENT\n")
	existing := "# My notes\n\n" +
		agent.InstructionsMarkerBegin + "\nOLD CONTENT\n" + agent.InstructionsMarkerEnd + "\n\n" +
		"More of my notes.\n"
	path := filepath.Join(stateProj, "claude/CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	s := string(data)
	if strings.Contains(s, "OLD CONTENT") {
		t.Errorf("OLD CONTENT should be replaced:\n%s", s)
	}
	if !strings.Contains(s, "NEW CONTENT") {
		t.Errorf("NEW CONTENT missing:\n%s", s)
	}
	if !strings.Contains(s, "# My notes") {
		t.Errorf("user text above markers lost:\n%s", s)
	}
	if !strings.Contains(s, "More of my notes.") {
		t.Errorf("user text below markers lost:\n%s", s)
	}
	// Exactly one marker pair (no accumulation across re-runs).
	if strings.Count(s, agent.InstructionsMarkerBegin) != 1 || strings.Count(s, agent.InstructionsMarkerEnd) != 1 {
		t.Errorf("marker pair should appear exactly once:\n%s", s)
	}
}

func TestInjectAgentInstructionsSourceMissingIsNoop(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "")
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Errorf("source missing should not error: %v", err)
	}
	for _, rel := range agent.AgentInstructionTargets {
		if _, err := os.Stat(filepath.Join(stateProj, rel)); !os.IsNotExist(err) {
			t.Errorf("%s should not be created when source is missing", rel)
		}
	}
}

func TestInjectAgentInstructionsRefreshesContentAcrossSourceChanges(t *testing.T) {
	stateProj, dockerDir := instructionsTestEnv(t, "v1 content\n")
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}
	// Change the source and re-inject; existing files should be refreshed.
	src := filepath.Join(dockerDir, "..", "defaults", "agent-instructions.md")
	if err := os.WriteFile(src, []byte("v2 content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agent.InjectAgentInstructions(stateProj, dockerDir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range agent.AgentInstructionTargets {
		data, _ := os.ReadFile(filepath.Join(stateProj, rel))
		s := string(data)
		if strings.Contains(s, "v1 content") {
			t.Errorf("%s still has v1 content:\n%s", rel, s)
		}
		if !strings.Contains(s, "v2 content") {
			t.Errorf("%s missing v2 content:\n%s", rel, s)
		}
	}
}
