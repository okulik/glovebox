package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/okulik/glovebox/internal/dockerx"
)

// Executor abstracts running external commands.
type Executor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemExecutor runs commands via os/exec against the host's PATH.
type SystemExecutor struct{}

func (SystemExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// RunStreaming is the optional Streamer hook: it wires the command's stdout
// and stderr directly to the caller's terminal.
func (SystemExecutor) RunStreaming(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Executor abstracts running external commands while supporting input/output streaming.
type Streamer interface {
	RunStreaming(ctx context.Context, name string, args ...string) error
}

// EnsureSpec holds the inputs to Ensure. Docker is the SDK-backed client
// used for all container/network operations; Create supplies the configs
// fed into BuildCreateConfig.
type EnsureSpec struct {
	Docker dockerx.ControllerClient
	Create CreateSpec
}

// Ensure guarantees that the agent container for spec.Create.PID exists and is
// running, and is attached to the per-project stack network if that network
// exists.
func Ensure(ctx context.Context, spec EnsureSpec) error {
	if spec.Docker == nil {
		return errors.New("agent.Ensure: Docker (dockerx.Client) is required")
	}
	cname := "glovebox-agent-" + spec.Create.PID

	subdirs := []string{"claude", "codex", "opencode", "pi", "gemini", "aider", "hermes"}
	for _, s := range subdirs {
		if err := os.MkdirAll(filepath.Join(spec.Create.StateProjDir, s), 0o755); err != nil {
			return fmt.Errorf("create state subdir %s: %w", s, err)
		}
	}

	// Inject glovebox guidance into each agent's conventional instruction
	// file (state/<pid>/{claude/CLAUDE.md,codex/AGENTS.md,gemini/GEMINI.md}).
	if err := InjectAgentInstructions(spec.Create.StateProjDir, spec.Create.DockerDir); err != nil {
		return fmt.Errorf("inject agent instructions: %w", err)
	}

	claudeDir := filepath.Join(spec.Create.StateProjDir, "claude")
	claudeJSON := filepath.Join(claudeDir, ".claude.json")
	if _, err := os.Stat(claudeJSON); os.IsNotExist(err) {
		if err := os.WriteFile(claudeJSON, []byte("{}\n"), 0o600); err != nil {
			return fmt.Errorf("seed .claude.json: %w", err)
		}
	}

	// Seed Claude defaults (settings.json, statusline-command.sh) on first
	// creation of the per-project claude state dir. Files that already exist
	// are left alone so user edits survive container recreation.
	defaultsDir := filepath.Join(spec.Create.DockerDir, "..", "defaults", "claude")
	for _, f := range []struct {
		name string
		mode os.FileMode
	}{
		{"settings.json", 0o644},
		{"statusline-command.sh", 0o755},
	} {
		// Containment: confine the write to claudeDir. StateProjDir is
		// taint-derived (built from GBX_* env vars on the host).
		dst, err := UnderBase(claudeDir, filepath.Join(claudeDir, f.name))
		if err != nil {
			return fmt.Errorf("seed claude/%s: %w", f.name, err)
		}

		if _, statErr := os.Stat(dst); statErr == nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(defaultsDir, f.name))
		if err != nil {
			continue // missing source is not fatal - older repos may not ship it
		}

		//nolint:gosec // G703: dst is validated by UnderBase above
		if err := os.WriteFile(dst, data, f.mode); err != nil {
			return fmt.Errorf("seed claude/%s: %w", f.name, err)
		}
	}

	id, _, err := spec.Docker.ContainerByName(ctx, cname)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	if id == "" {
		cfg, hostCfg, netCfg, name := BuildCreateConfig(spec.Create)
		if _, err := spec.Docker.CreateContainerRaw(ctx, name, cfg, hostCfg, netCfg); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
	}

	if err := spec.Docker.StartContainer(ctx, cname); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	stacknet := "glovebox-stack-" + spec.Create.PID
	if _, exists, nerr := spec.Docker.NetworkContainerCount(ctx, stacknet); nerr == nil && exists {
		// ConnectNetwork already swallows "already attached" so this stays
		// idempotent across re-runs.
		_ = spec.Docker.ConnectNetwork(ctx, cname, stacknet)
	}

	return nil
}

// Remove forcibly stops and removes the agent container for pid. A missing
// container is not treated as an error.
func Remove(ctx context.Context, dc dockerx.ControllerClient, pid string) error {
	if dc == nil {
		return errors.New("agent.Remove: dockerx.Client is required")
	}

	// RemoveContainer with force=true is best-effort; a NotFound or
	// transient daemon error shouldn't bubble up to the user.
	_ = dc.RemoveContainer(ctx, "glovebox-agent-"+pid, true)
	return nil
}
