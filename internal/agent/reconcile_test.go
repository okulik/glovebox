package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDeepMergeJSON_UnionsAllowList(t *testing.T) {
	dst := map[string]any{"permissions": map[string]any{"allow": []any{"Read(//data/**)"}}}
	src := map[string]any{"permissions": map[string]any{"allow": []any{"Bash(gbx-stack *)", "Read(//data/**)"}}}
	got := deepMergeJSON(dst, src).(map[string]any)
	allow := got["permissions"].(map[string]any)["allow"].([]any)
	// user entry kept first, new entry appended, exact dup not duplicated.
	if len(allow) != 2 {
		t.Fatalf("allow = %v, want 2 entries", allow)
	}
	if allow[0] != "Read(//data/**)" || allow[1] != "Bash(gbx-stack *)" {
		t.Errorf("allow order/content = %v", allow)
	}
}

func TestDeepMergeJSON_ScalarUserWinsFillsMissing(t *testing.T) {
	dst := map[string]any{"model": "opus"}
	src := map[string]any{"model": "sonnet", "theme": "dark"}
	got := deepMergeJSON(dst, src).(map[string]any)
	if got["model"] != "opus" {
		t.Errorf("model = %v, user value must win", got["model"])
	}
	if got["theme"] != "dark" {
		t.Errorf("theme = %v, missing key must be filled", got["theme"])
	}
}

func TestDeepMergeJSON_Idempotent(t *testing.T) {
	dst := map[string]any{"a": float64(1), "list": []any{"x"}}
	src := map[string]any{"a": float64(2), "list": []any{"x", "y"}, "b": "new"}
	once := deepMergeJSON(dst, src)
	twice := deepMergeJSON(once, src)
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("not idempotent:\n once=%v\n twice=%v", once, twice)
	}
}

func writeReconcileDefaults(t *testing.T, base string) string {
	t.Helper()
	dockerDir := filepath.Join(base, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defClaude := filepath.Join(base, "defaults", "claude")
	if err := os.MkdirAll(defClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defClaude, "settings.json"),
		[]byte(`{"model":"sonnet","permissions":{"allow":["Bash(gbx-stack *)"]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defClaude, "statusline-command.sh"),
		[]byte("#!/bin/bash\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "defaults", "agent-instructions.md"),
		[]byte("summary text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dockerDir
}

func TestReconcileState_SeedsAndMerges(t *testing.T) {
	base := t.TempDir()
	dockerDir := writeReconcileDefaults(t, base)
	stateProjDir := filepath.Join(base, "state")
	if err := os.MkdirAll(filepath.Join(stateProjDir, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing user settings: custom model + the user's own allow entry.
	if err := os.WriteFile(filepath.Join(stateProjDir, "claude", "settings.json"),
		[]byte(`{"model":"opus","permissions":{"allow":["Read(//data/**)"]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-create a STALE statusline so the overwrite path is exercised.
	if err := os.WriteFile(filepath.Join(stateProjDir, "claude", "statusline-command.sh"),
		[]byte("#!/bin/bash\necho OLD\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	changed, err := ReconcileState(stateProjDir, dockerDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) == 0 {
		t.Errorf("expected changed files on first run")
	}

	got, _ := os.ReadFile(filepath.Join(stateProjDir, "claude", "settings.json"))
	var m map[string]any
	if err = json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "opus" {
		t.Errorf("model = %v, user value must win", m["model"])
	}
	allow := m["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 2 {
		t.Errorf("allow = %v, want union of 2", allow)
	}
	if _, serr := os.Stat(filepath.Join(stateProjDir, "claude", "statusline-command.sh")); serr != nil {
		t.Errorf("statusline not seeded: %v", serr)
	}
	cm, err := os.ReadFile(filepath.Join(stateProjDir, "claude", "CLAUDE.md"))
	if err != nil || !strings.Contains(string(cm), "summary text") {
		t.Errorf("CLAUDE.md not refreshed: err=%v content=%s", err, cm)
	}
	sl, _ := os.ReadFile(filepath.Join(stateProjDir, "claude", "statusline-command.sh"))
	if !strings.Contains(string(sl), "echo hi") {
		t.Errorf("statusline not overwritten: %s", sl)
	}
	if !slices.Contains(changed, "claude/settings.json") || !slices.Contains(changed, "claude/statusline-command.sh") {
		t.Errorf("changed missing expected entries: %v", changed)
	}

	// Idempotent: a second run changes nothing.
	changed2, err := ReconcileState(stateProjDir, dockerDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed2) != 0 {
		t.Errorf("second run changed = %v, want none", changed2)
	}
}

func TestReconcileState_InvalidExistingSettings(t *testing.T) {
	base := t.TempDir()
	dockerDir := writeReconcileDefaults(t, base)
	stateProjDir := filepath.Join(base, "state")
	if err := os.MkdirAll(filepath.Join(stateProjDir, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateProjDir, "claude", "settings.json"),
		[]byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileState(stateProjDir, dockerDir); err == nil {
		t.Fatal("expected error on invalid existing settings.json")
	}
}
