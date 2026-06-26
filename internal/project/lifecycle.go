package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/hostconfig"
	"github.com/okulik/glovebox/internal/state"
)

// EnsureAgentFn is the signature for the agent-ensure callback injected into New.
type EnsureAgentFn func(ctx context.Context, dc dockerx.ControllerClient, pid, workspace, libexec, stateDir string) error

// RemoveAgentFn mirrors EnsureAgentFn for the rm path.
type RemoveAgentFn func(ctx context.Context, dc dockerx.ControllerClient, pid string) error

// NewSpec carries the inputs to New.
type NewSpec struct {
	Docker      dockerx.ControllerClient
	EnsureAgent EnsureAgentFn
	Workspace   string
	ConfigDir   string
	LibExec     string
}

// NewResult reports what New decided.
type NewResult struct {
	PID               string
	WorkspaceAbs      string
	SetAsDefault      bool
	AlreadyRegistered bool
}

// New ports cmd_project_new: validate path, bootstrap, compute pid, write
// workspace-path, ensure agent, set default if none exists.
func New(ctx context.Context, spec NewSpec) (NewResult, error) {
	abs, err := filepath.Abs(spec.Workspace)
	if err != nil {
		return NewResult{}, fmt.Errorf("abs path: %w", err)
	}
	if fi, statErr := os.Stat(abs); statErr != nil {
		return NewResult{}, fmt.Errorf("not a directory: %s", abs)
	} else if !fi.IsDir() {
		return NewResult{}, fmt.Errorf("not a directory: %s", abs)
	}
	// Match bash `readlink -f`: resolve symlinks so the recorded workspace
	// path matches what Hash sees (e.g. /var → /private/var on macOS).
	if resolved, slErr := filepath.EvalSymlinks(abs); slErr == nil {
		abs = resolved
	}
	if bootErr := hostconfig.Bootstrap(spec.LibExec, spec.ConfigDir); bootErr != nil {
		return NewResult{}, fmt.Errorf("bootstrap: %w", bootErr)
	}
	pid, err := Hash(abs)
	if err != nil {
		return NewResult{}, fmt.Errorf("compute pid: %w", err)
	}
	stateDir := filepath.Join(spec.ConfigDir, config.StatePath)
	projDir := filepath.Join(stateDir, config.ProjectsPath, pid)
	wspath := filepath.Join(projDir, config.WorkspacePath)

	res := NewResult{PID: pid, WorkspaceAbs: abs}

	if _, err := os.Stat(wspath); err == nil {
		res.AlreadyRegistered = true
	} else {
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			return NewResult{}, fmt.Errorf("mkdir project state: %w", err)
		}
		if err := os.WriteFile(wspath, []byte(abs+"\n"), 0o600); err != nil {
			return NewResult{}, fmt.Errorf("write workspace-path: %w", err)
		}
	}

	if spec.EnsureAgent != nil {
		if err := spec.EnsureAgent(ctx, spec.Docker, pid, abs, spec.LibExec, stateDir); err != nil {
			return NewResult{}, err
		}
	}

	curPID, _ := state.ActivePID(spec.ConfigDir)
	if curPID == "" {
		demoted, _ := state.ConsumeRemovedDefault(spec.ConfigDir)
		if demoted != pid {
			if err := state.WriteActive(spec.ConfigDir, pid, abs); err != nil {
				return NewResult{}, fmt.Errorf("set default: %w", err)
			}
			res.SetAsDefault = true
		}
	}
	return res, nil
}

// Use ports cmd_project_use: resolve prefix, write active-project.
func Use(configDir, prefix string) error {
	stateDir := filepath.Join(configDir, config.StatePath)
	pid, err := Resolve(stateDir, prefix)
	if err != nil {
		return err
	}
	wspath := filepath.Join(stateDir, config.ProjectsPath, pid, config.WorkspacePath)
	ws, err := os.ReadFile(wspath)
	if err != nil {
		return fmt.Errorf("project %s has no recorded workspace", pid)
	}
	wsStr := stringTrimTrailingNewline(string(ws))
	if err := state.WriteActive(configDir, pid, wsStr); err != nil {
		return err
	}
	_ = state.ClearRemovedDefault(configDir)
	return nil
}

func stringTrimTrailingNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

type RemoveSpec struct {
	Docker      dockerx.ControllerClient
	Confirm     func() bool
	RemoveAgent RemoveAgentFn
	Prefix      string
	ConfigDir   string
	DeleteState bool
}

func Remove(ctx context.Context, spec RemoveSpec) error {
	stateDir := filepath.Join(spec.ConfigDir, config.StatePath)
	pid, err := Resolve(stateDir, spec.Prefix)
	if err != nil {
		return err
	}
	if spec.Confirm != nil && !spec.Confirm() {
		return errors.New("Aborted.")
	}
	if spec.RemoveAgent != nil {
		if err := spec.RemoveAgent(ctx, spec.Docker, pid); err != nil {
			return err
		}
	}
	if spec.DeleteState {
		if err := os.RemoveAll(filepath.Join(stateDir, config.ProjectsPath, pid)); err != nil {
			return fmt.Errorf("remove state: %w", err)
		}
	}
	curPID, _ := state.ActivePID(spec.ConfigDir)
	if curPID == pid {
		_ = os.Remove(filepath.Join(spec.ConfigDir, config.ActiveProjectPath))
		_ = state.MarkRemovedDefault(spec.ConfigDir, pid)
	}
	return nil
}
