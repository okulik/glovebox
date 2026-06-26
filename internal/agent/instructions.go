package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	AgentInstructionsDoc    = "agent-instructions.md"
	DefaultsPath            = "defaults"
	InstructionsMarkerBegin = "<!-- glovebox-instructions-begin -->"
	InstructionsMarkerEnd   = "<!-- glovebox-instructions-end -->"
)

// AgentInstructionTargets are the per-agent instruction files into which we
// inject glovebox guidance.
var AgentInstructionTargets = []string{
	"claude/CLAUDE.md",
	"codex/AGENTS.md",
	"gemini/GEMINI.md",
}

// InjectAgentInstructions writes (or refreshes) a marker-wrapped block of
// glovebox guidance into each agent's conventional instruction file.
func InjectAgentInstructions(stateProjDir, dockerDir string) error {
	src := filepath.Join(dockerDir, "..", DefaultsPath, AgentInstructionsDoc)
	canonical, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", src, err)
	}
	block := buildInstructionsBlock(canonical)
	for _, rel := range AgentInstructionTargets {
		if err := writeInstructionsFile(filepath.Join(stateProjDir, rel), block); err != nil {
			return err
		}
	}
	return nil
}

// buildInstructionsBlock wraps the canonical content in the marker pair with
// a trailing newline so concatenation onto existing content stays tidy.
func buildInstructionsBlock(canonical []byte) string {
	var b strings.Builder
	b.WriteString(InstructionsMarkerBegin)
	b.WriteString("\n")
	b.Write(bytes.TrimRight(canonical, "\n"))
	b.WriteString("\n")
	b.WriteString(InstructionsMarkerEnd)
	b.WriteString("\n")
	return b.String()
}

// writeInstructionsFile ensures path's content contains block.
func writeInstructionsFile(path, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var out string
	switch {
	case len(existing) == 0:
		out = block
	case bytes.Contains(existing, []byte(InstructionsMarkerBegin)) &&
		bytes.Contains(existing, []byte(InstructionsMarkerEnd)):
		out = replaceMarkedBlock(string(existing), block)
	default:
		sep := "\n\n"
		if bytes.HasSuffix(existing, []byte("\n")) {
			sep = "\n"
		}
		out = string(existing) + sep + block
	}

	if out == string(existing) {
		return nil
	}
	return WriteAtomic(path, []byte(out), 0o644)
}

// replaceMarkedBlock swaps the bytes between (and including) the begin/end
// markers with block.
func replaceMarkedBlock(existing, block string) string {
	beginIdx := strings.Index(existing, InstructionsMarkerBegin)
	endIdx := strings.Index(existing, InstructionsMarkerEnd)
	if beginIdx < 0 || endIdx < 0 || endIdx < beginIdx {
		return existing + "\n\n" + block
	}
	endIdx += len(InstructionsMarkerEnd)
	if endIdx < len(existing) && existing[endIdx] == '\n' {
		endIdx++
	}
	return existing[:beginIdx] + block + existing[endIdx:]
}
