package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/okulik/glovebox/internal/fsx"
)

// instructionsMarkerBegin / instructionsMarkerEnd delimit the glovebox-owned
// block within each agent's instruction file. The content between the markers
// is refreshed on every Ensure; anything outside (user notes, project rules)
// is preserved verbatim.
const (
	instructionsMarkerBegin = "<!-- glovebox-instructions-begin -->"
	instructionsMarkerEnd   = "<!-- glovebox-instructions-end -->"
)

// agentInstructionTargets are the per-agent instruction files into which we
// inject glovebox guidance. The paths are relative to stateProjDir; each one
// is bind-mounted into the agent container at the location its CLI expects.
//
//	state/<pid>/claude/CLAUDE.md   →  /home/gbx/.claude/CLAUDE.md
//	state/<pid>/codex/AGENTS.md    →  /home/gbx/.codex/AGENTS.md
//	state/<pid>/gemini/GEMINI.md   →  /home/gbx/.gemini/GEMINI.md
var agentInstructionTargets = []string{
	"claude/CLAUDE.md",
	"codex/AGENTS.md",
	"gemini/GEMINI.md",
}

// injectAgentInstructions writes (or refreshes) a marker-wrapped block of
// glovebox guidance into each agent's conventional instruction file. The
// content inlined into the block comes from defaults/agent-instructions.md
// - a deliberately short pointer to the full operating docs that live at
// /etc/glovebox/docker-sandbox.md, and /etc/glovebox/proxy-sandbox.md
// inside the container (bind-mounted from defaults/docker-sandbox.md on
// the host).
//
// Inlining only the summary keeps every agent session's system-prompt cost
// small while still ensuring the most safety-critical bit (the egress 451 +
// X-Glovebox-Egress signal) is in front of the agent before it makes its
// first request. Full doc stays as a live, always-current bind mount.
//
// Idempotent: re-running with the same source content writes the same bytes
// and is a no-op for downstream consumers (mtime is left alone when the file
// is already up to date). Updates to the source replace only the content
// between markers - user edits outside the block survive.
//
// A missing source file is treated as "nothing to inject" rather than an
// error, so older glovebox checkouts without the summary keep working.
func injectAgentInstructions(stateProjDir, dockerDir string) error {
	src := filepath.Join(dockerDir, "..", "defaults", "agent-instructions.md")
	canonical, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", src, err)
	}
	block := buildInstructionsBlock(canonical)
	for _, rel := range agentInstructionTargets {
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
	b.WriteString(instructionsMarkerBegin)
	b.WriteString("\n")
	b.Write(bytes.TrimRight(canonical, "\n"))
	b.WriteString("\n")
	b.WriteString(instructionsMarkerEnd)
	b.WriteString("\n")
	return b.String()
}

// writeInstructionsFile ensures path's content contains block. Modes:
//
//   - file missing:                   write block only
//   - existing file with markers:     replace text between markers, keep rest
//   - existing file without markers:  append block at the end (newline-safe)
//
// The write is skipped when the resulting bytes match what's already on disk,
// so re-invocations don't churn the file's mtime.
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
	case bytes.Contains(existing, []byte(instructionsMarkerBegin)) &&
		bytes.Contains(existing, []byte(instructionsMarkerEnd)):
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
	return fsx.WriteAtomic(path, []byte(out), 0o644)
}

// replaceMarkedBlock swaps the bytes between (and including) the begin/end
// markers with block. If the markers are present but out of order, the
// function falls back to appending - better to over-tag than to mangle.
func replaceMarkedBlock(existing, block string) string {
	beginIdx := strings.Index(existing, instructionsMarkerBegin)
	endIdx := strings.Index(existing, instructionsMarkerEnd)
	if beginIdx < 0 || endIdx < 0 || endIdx < beginIdx {
		return existing + "\n\n" + block
	}
	endIdx += len(instructionsMarkerEnd)
	// Eat one trailing newline so we don't accumulate blank lines on repeat.
	if endIdx < len(existing) && existing[endIdx] == '\n' {
		endIdx++
	}
	return existing[:beginIdx] + block + existing[endIdx:]
}
