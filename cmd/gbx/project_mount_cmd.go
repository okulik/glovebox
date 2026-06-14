package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/okulik/glovebox/internal/agent"
)

// ProjectMountCmd is the `gbx mount ...` group. All subcommands target the
// pid resolved by targetPID() (`-p <id>` if set, else the active project).
type ProjectMountCmd struct {
	Ls    ProjectMountLsCmd    `cmd:"" help:"List the project's extra mounts."`
	Apply ProjectMountApplyCmd `cmd:"" help:"Recreate the agent container so mount changes take effect."`
	Add   ProjectMountAddCmd   `cmd:"" help:"Append a host:container[:rw|ro] bind mount."`
	Rm    ProjectMountRmCmd    `cmd:"" help:"Remove a mount by host or container path."`
}

// projectMountStateDir resolves the per-project state dir for the current
// invocation's target pid, ensuring the dir exists (mount add may run before
// the first agent is created).
func projectMountStateDir() (string, error) {
	pid, err := targetPID()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(stateDirFromEnv(), "projects", pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create state dir for %s: %w", pid, err)
	}
	return dir, nil
}

// --- gbx mount add <spec> ---

type ProjectMountAddCmd struct {
	Spec string `arg:"" help:"<host>[:<container>][:rw|ro] (bare host defaults container=/mnt/<basename> mode=rw)."`
}

func (c *ProjectMountAddCmd) Run(kctx *kong.Context) error {
	dir, err := projectMountStateDir()
	if err != nil {
		return err
	}
	m, err := agent.ParseMountSpec(c.Spec)
	if err != nil {
		return err
	}
	existing, err := agent.ReadMounts(dir)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.Container == m.Container {
			return fmt.Errorf("container path %s already mounted from %s", e.Container, e.Host)
		}
	}
	existing = append(existing, m)
	if err := agent.WriteMounts(dir, existing); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Added %s. Run `gbx mount apply` to recreate the agent container.\n", m.String())
	return nil
}

// --- gbx mount rm <query> ---

type ProjectMountRmCmd struct {
	Query string `arg:"" help:"Host path or container path of a previously added mount."`
}

func (c *ProjectMountRmCmd) Run(kctx *kong.Context) error {
	dir, err := projectMountStateDir()
	if err != nil {
		return err
	}
	existing, err := agent.ReadMounts(dir)
	if err != nil {
		return err
	}
	// Try matching the query literally against host or container first; if
	// that misses, try the symlink-resolved host (mounts.txt stores the
	// resolved form, so users typing the unresolved path still work).
	resolved := c.Query
	if r, err := filepath.EvalSymlinks(c.Query); err == nil {
		resolved = r
	}
	var kept []agent.Mount
	removed := 0
	for _, m := range existing {
		if m.Host == c.Query || m.Container == c.Query || m.Host == resolved {
			removed++
			continue
		}
		kept = append(kept, m)
	}
	if removed == 0 {
		return fmt.Errorf("no mount matched %q", c.Query)
	}
	if err := agent.WriteMounts(dir, kept); err != nil {
		return err
	}
	noun := "entry"
	if removed > 1 {
		noun = "entries"
	}
	fmt.Fprintf(kctx.Stdout, "Removed %d %s. Run `gbx mount apply` to recreate the agent container.\n", removed, noun)
	return nil
}

// --- gbx mount ls ---

type ProjectMountLsCmd struct{}

func (c *ProjectMountLsCmd) Run(kctx *kong.Context) error {
	dir, err := projectMountStateDir()
	if err != nil {
		return err
	}
	mounts, err := agent.ReadMounts(dir)
	if err != nil {
		return err
	}
	for _, m := range mounts {
		fmt.Fprintln(kctx.Stdout, m.String())
	}
	return nil
}

// --- gbx mount apply ---

type ProjectMountApplyCmd struct{}

func (c *ProjectMountApplyCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	pid, err := targetPID()
	if err != nil {
		return err
	}
	wsfile := filepath.Join(stateDirFromEnv(), "projects", pid, "workspace-path")
	data, err := os.ReadFile(wsfile)
	if err != nil {
		return fmt.Errorf("Project %s has no recorded workspace.", pid)
	}
	ws := strings.TrimRight(string(data), "\r\n")
	cname := "glovebox-agent-" + pid
	ctx := context.Background()
	if err := hostDocker.ForceRemoveContainer(ctx, cname); err != nil {
		return fmt.Errorf("failed to force remove container '%s': %w", cname, err)
	}
	libexec := os.Getenv("GBX_LIBEXEC")
	if err := ensureAgentFn(ctx, hostClient, pid, ws, libexec, stateDirFromEnv()); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Recreated %s with the current mount set.\n", cname)
	return nil
}
