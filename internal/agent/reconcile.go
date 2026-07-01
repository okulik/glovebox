package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReconcileState brings a project's glovebox-managed on-disk state up to date
// with the current shipped defaults, without recreating the container:
//
//   - claude/settings.json          deep-merged from defaults (deepMergeJSON)
//   - claude/statusline-command.sh  overwritten from defaults
//   - claude/CLAUDE.md, codex/AGENTS.md, gemini/GEMINI.md
//     instruction block refreshed (injectAgentInstructions)
//
// It returns the changed file paths (relative to stateProjDir) for reporting,
// detecting instruction-file changes by comparing bytes before/after the
// refresh. A missing source default is skipped, not fatal. An existing
// settings.json that is not valid JSON is an error.
func ReconcileState(stateProjDir, dockerDir string) ([]string, error) {
	var changed []string
	defaultsClaude := filepath.Join(dockerDir, "..", "defaults", "claude")
	claudeDir := filepath.Join(stateProjDir, "claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", claudeDir, err)
	}

	settingsChanged, err := reconcileSettings(
		filepath.Join(claudeDir, "settings.json"),
		filepath.Join(defaultsClaude, "settings.json"))
	if err != nil {
		return nil, err
	}
	if settingsChanged {
		changed = append(changed, "claude/settings.json")
	}

	slChanged, err := reconcileOverwrite(
		filepath.Join(claudeDir, "statusline-command.sh"),
		filepath.Join(defaultsClaude, "statusline-command.sh"), 0o755)
	if err != nil {
		return nil, err
	}
	if slChanged {
		changed = append(changed, "claude/statusline-command.sh")
	}

	instrFiles := AgentInstructionTargets
	before := make(map[string][]byte, len(instrFiles))
	for _, rel := range instrFiles {
		before[rel], _ = os.ReadFile(filepath.Join(stateProjDir, rel))
	}
	if err := InjectAgentInstructions(stateProjDir, dockerDir); err != nil {
		return nil, err
	}
	for _, rel := range instrFiles {
		after, _ := os.ReadFile(filepath.Join(stateProjDir, rel))
		if !bytes.Equal(before[rel], after) {
			changed = append(changed, rel)
		}
	}
	return changed, nil
}

// reconcileSettings deep-merges the default settings file into the existing one
// and writes the result (canonical JSON, alphabetical keys) back when it
// differs. Returns whether the file changed. A missing default is a no-op
// (false, nil). An existing file that is not valid JSON is an error. The output
// is always re-marshaled, so the merge is idempotent across reruns.
func reconcileSettings(dstPath, srcPath string) (bool, error) {
	srcRaw, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", srcPath, err)
	}
	var src any
	if err = json.Unmarshal(srcRaw, &src); err != nil {
		return false, fmt.Errorf("parse default %s: %w", srcPath, err)
	}

	existing, readErr := os.ReadFile(dstPath)
	fileExists := !os.IsNotExist(readErr)
	if readErr != nil && fileExists {
		return false, fmt.Errorf("read %s: %w", dstPath, readErr)
	}

	var merged any
	if !fileExists {
		merged = src
	} else {
		var dst any
		if uerr := json.Unmarshal(existing, &dst); uerr != nil {
			return false, fmt.Errorf("parse %s: %w", dstPath, uerr)
		}
		merged = DeepMergeJSON(dst, src)
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", dstPath, err)
	}
	out = append(out, '\n')
	if bytes.Equal(out, existing) {
		return false, nil
	}
	if err := WriteAtomic(dstPath, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// reconcileOverwrite copies src over dst when their bytes differ. A missing src
// is a no-op. Returns whether dst changed.
func reconcileOverwrite(dstPath, srcPath string, mode os.FileMode) (bool, error) {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", srcPath, err)
	}
	existing, err := os.ReadFile(dstPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", dstPath, err)
	}
	if bytes.Equal(src, existing) {
		return false, nil
	}
	if err := WriteAtomic(dstPath, src, mode); err != nil {
		return false, err
	}
	return true, nil
}
