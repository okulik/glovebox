package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/pathx"
)

// Executor abstracts running external commands so the per-agent
// install/update orchestration can be unit-tested without a real Docker
// daemon. Production uses SystemExecutor (defined in this file); tests
// inject a fake. Note: Ensure/Remove no longer use Executor - those go
// through dockerx.Client. Executor survives for Install/Update only.
type Executor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemExecutor runs commands via os/exec against the host's PATH.
type SystemExecutor struct{}

func (SystemExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// RunStreaming is the optional Streamer hook: it wires the command's stdout
// and stderr directly to the caller's terminal so install/update progress is
// visible in real time instead of buffered until the command exits.
func (SystemExecutor) RunStreaming(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Streamer is an optional interface that runSpec uses to stream output from
// the install/update commands instead of buffering it. Production
// SystemExecutor implements this; test fakes typically do not, in which case
// runSpec falls back to the buffered Executor.Run path.
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
// exists. It is idempotent: re-running on a healthy environment is a no-op
// beyond a couple of container/network inspect probes.
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
	// Idempotent: the source between markers is refreshed, user text outside
	// is preserved. Done before container create so a fresh container sees
	// the guidance on first session.
	if err := injectAgentInstructions(spec.Create.StateProjDir, spec.Create.DockerDir); err != nil {
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
		dst, err := pathx.UnderBase(claudeDir, filepath.Join(claudeDir, f.name))
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
		//nolint:gosec // G703: dst is validated by pathx.UnderBase above
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
