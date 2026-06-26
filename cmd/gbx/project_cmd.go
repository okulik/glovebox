package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/hostconfig"
	"github.com/okulik/glovebox/internal/plugin"
	"github.com/okulik/glovebox/internal/project"
	"github.com/okulik/glovebox/internal/state"
)

const (
	ProjectNewTimeout = time.Second * 30
)

// removeAgentFn is the indirection used by ProjectRmCmd so tests can swap the
// real agent.Remove for a stub. Production keeps the closure that wraps
// agent.Remove.
var removeAgentFn project.RemoveAgentFn = func(ctx context.Context, dc dockerx.ControllerClient, pid string) error {
	return agent.Remove(ctx, dc, pid)
}

type ProjectUseCmd struct {
	IDOrPrefix string `arg:"" help:"Project pid or prefix."`
}

func (c *ProjectUseCmd) Run(kctx *kong.Context) error {
	cfg := config.GbxFromEnv().ConfigDir
	if err := project.Use(cfg, c.IDOrPrefix); err != nil {
		return err
	}
	pid, err := state.ActivePID(cfg)
	if err != nil {
		return fmt.Errorf("can't read active pid: %w", err)
	}
	ws, err := state.ActivePath(cfg)
	if err != nil {
		return fmt.Errorf("can't read active path: %w", err)
	}
	fmt.Fprintf(kctx.Stdout, "Default project: %s (%s).\n", pid, ws)
	return nil
}

// ellipsizeLeft caps s to n runes (not bytes) by dropping from the left
// and prefixing "...". Paths like /Users/orest/dev/projects/foo are most
// recognizable by their basename, so trimming the front preserves the
// project name. For n<=3 (no room for an ellipsis), the tail is returned
// raw and may still exceed n.
func ellipsizeLeft(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[len(runes)-n:])
	}
	return "..." + string(runes[len(runes)-(n-3):])
}

// truncRunes truncates s to at most n runes (not bytes), matching the
// printf %.*s behavior we'd want in docker's --format strings.
func truncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

type ProjectRmCmd struct {
	IDOrPrefix  string `arg:"" optional:"" help:"Project pid or prefix (omit when --all)."`
	DeleteState bool   `name:"delete-state" help:"Also delete state/projects/<pid>/. Without this flag the state dir is preserved so re-creating the project picks up where it left off."`
	Yes         bool   `short:"y" name:"yes" help:"Skip the y/N prompt."`
	All         bool   `name:"all" help:"Remove every registered project."`
}

func (c *ProjectRmCmd) Run(kctx *kong.Context) error {
	cfg := config.GbxFromEnv().ConfigDir
	stateDir := filepath.Join(cfg, config.StatePath)
	if c.All && c.IDOrPrefix != "" {
		return errors.New("gbx rm: --all conflicts with an explicit pid argument")
	}
	if !c.All && c.IDOrPrefix == "" {
		return errors.New("gbx rm: missing pid (or pass --all)")
	}

	// Build the list of pids to remove.
	var pids []string
	if c.All {
		entries, err := os.ReadDir(filepath.Join(stateDir, config.ProjectsPath))
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(kctx.Stdout, "No projects to remove.")
				return nil
			}
			return fmt.Errorf("read projects dir: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				pids = append(pids, e.Name())
			}
		}
		if len(pids) == 0 {
			fmt.Fprintln(kctx.Stdout, "No projects to remove.")
			return nil
		}
		if !c.Yes && !confirmYN(kctx.Stderr, fmt.Sprintf("Remove %d projects (%s)? [y/N] ", len(pids), strings.Join(pids, ", "))) {
			return errors.New("Aborted.")
		}
	} else {
		pids = []string{c.IDOrPrefix}
	}

	// Per-pid confirm: bulk mode already prompted once; single-pid mode uses
	// the original interactive prompt.
	for _, pid := range pids {
		confirm := func() bool {
			if c.Yes || c.All {
				return true
			}
			return confirmYN(kctx.Stderr, fmt.Sprintf("Remove project %s? [y/N] ", pid))
		}
		if err := project.Remove(context.Background(), project.RemoveSpec{
			Docker:      projectClient(),
			Prefix:      pid,
			ConfigDir:   cfg,
			DeleteState: c.DeleteState,
			Confirm:     confirm,
			RemoveAgent: removeAgentFn,
		}); err != nil {
			return err
		}
		fmt.Fprintf(kctx.Stdout, "Removed project %s.\n", pid)
	}
	return nil
}

// ensureAgentFn is the indirection so tests can swap a stub. Production wraps
// agent.Ensure with the correct CreateSpec built from env and project state.
var ensureAgentFn project.EnsureAgentFn = func(ctx context.Context, dc dockerx.ControllerClient, pid, ws, libexec, stateDir string) error {
	if err := ensureAgentImage(libexec); err != nil {
		return fmt.Errorf("build agent image: %w", err)
	}
	stateProjDir := path.Join(stateDir, config.ProjectsPath, pid)
	mounts, err := agent.ReadMounts(stateProjDir)
	if err != nil {
		return fmt.Errorf("read project mounts: %w", err)
	}
	gcfg := config.GbxFromEnv()
	var labels map[string]string
	if gcfg.TestMode {
		labels = map[string]string{"io.glovebox.test": "1"}
	}
	image := plugin.SelectImage(ctx, hostDocker, gcfg.AgentImage, pid)
	return agent.Ensure(ctx, agent.EnsureSpec{
		Docker: dc,
		Create: agent.CreateSpec{
			PID:            pid,
			Workspace:      ws,
			Image:          image,
			StateProjDir:   stateProjDir,
			StateSharedDir: path.Join(stateDir, "shared"),
			DockerDir:      path.Join(libexec, "docker"),
			HostEnv:        envMap(),
			Mounts:         mounts,
			Labels:         labels,
		},
	})
}

func envMap() map[string]string {
	out := map[string]string{}
	for _, key := range agent.HostEnvVars {
		out[key] = os.Getenv(key)
	}
	return out
}

type ProjectNewCmd struct {
	HostPath string `arg:"" help:"Workspace path to register."`
}

func (c *ProjectNewCmd) Run(kctx *kong.Context) error {
	libexec := os.Getenv("GBX_LIBEXEC")
	if libexec == "" {
		return errors.New("GBX_LIBEXEC not set (should be set by bin/gbx)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), ProjectNewTimeout)
	defer cancel()

	// Bring the singleton stack up so the agent network exists before
	// docker create runs. Tests stub this out by setting GBX_SKIP_STACK_UP=1
	// - they don't have docker available and inject a fake EnsureAgent
	// instead.
	if os.Getenv("GBX_SKIP_STACK_UP") != "1" {
		if err := requireDocker(); err != nil {
			return err
		}
		// Seed config dir before compose Up: ensureStackUp reads
		// ${GBX_CONFIG_DIR}/.env. If the user just deleted ~/.config/glovebox,
		// we recreate it here from .env.example.
		if err := hostconfig.Bootstrap(libexec, config.GbxFromEnv().ConfigDir); err != nil {
			return err
		}
		if err := ensureStackUp(ctx); err != nil {
			return err
		}
	}

	res, err := project.New(ctx, project.NewSpec{
		Docker:      projectClient(),
		Workspace:   c.HostPath,
		ConfigDir:   config.GbxFromEnv().ConfigDir,
		LibExec:     libexec,
		EnsureAgent: ensureAgentFn,
	})
	if err != nil {
		return err
	}

	if res.AlreadyRegistered {
		fmt.Fprintf(kctx.Stdout, "Already registered as %s.\n", res.PID)
	} else if res.SetAsDefault {
		fmt.Fprintf(kctx.Stdout, "Registered project %s at %s. Set as default.\n", res.PID, res.WorkspaceAbs)
	} else {
		cur, err := state.ActivePID(config.GbxFromEnv().ConfigDir)
		if err != nil {
			return fmt.Errorf("can't read active pid: %w", err)
		}
		if cur == "" {
			fmt.Fprintf(kctx.Stdout, "Registered project %s at %s. (previously demoted; run `gbx use %s` to set as default)\n", res.PID, res.WorkspaceAbs, res.PID)
		} else {
			fmt.Fprintf(kctx.Stdout, "Registered project %s at %s. (default remains %s)\n", res.PID, res.WorkspaceAbs, cur)
		}
	}

	return nil
}
