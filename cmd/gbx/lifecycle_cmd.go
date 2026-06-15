package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/plugin"
	"github.com/okulik/glovebox/internal/stack"
)

// --- gbx up ---

// UpCmd brings the singleton egress-proxy + socket-proxy + stack-controller
// trio to a healthy steady state. Idempotent: no-op if egress-proxy is
// already running. Replaces the operator's reach for `docker compose up`.
type UpCmd struct{}

func (c *UpCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	if err := requireEnvFile(); err != nil {
		return err
	}
	if err := ensureStackUp(context.Background()); err != nil {
		return err
	}
	fmt.Fprintln(kctx.Stdout, "Stack is up.")
	return nil
}

// --- gbx start ---

type ProjectStartCmd struct {
	IDOrPrefix string `arg:"" optional:"" help:"Project pid or prefix; defaults to active."`
}

func (c *ProjectStartCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	ctx := context.Background()
	if err := ensureStackUp(ctx); err != nil {
		return err
	}
	pid, err := resolveProjectTarget(c.IDOrPrefix)
	if err != nil {
		return err
	}
	wsfile := filepath.Join(stateDirFromEnv(), projectsPath, pid, workspacePathFile)
	data, err := os.ReadFile(wsfile)
	if err != nil {
		return fmt.Errorf("Project %s has no recorded workspace.", pid)
	}
	ws := strings.TrimRight(string(data), "\r\n")
	libexec := os.Getenv("GBX_LIBEXEC")
	if err := ensureAgentFn(ctx, hostClient, pid, ws, libexec, stateDirFromEnv()); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Started glovebox-agent-%s.\n", pid)
	return nil
}

// --- gbx stop ---

type ProjectStopCmd struct {
	IDOrPrefix string `arg:"" optional:"" help:"Project pid or prefix; defaults to active."`
}

func (c *ProjectStopCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	pid, err := resolveProjectTarget(c.IDOrPrefix)
	if err != nil {
		return err
	}
	cname := "glovebox-agent-" + pid
	if err := hostDocker.StopContainer(context.Background(), cname); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Stopped %s.\n", cname)
	return nil
}

// --- gbx restart ---

type ProjectRestartCmd struct {
	IDOrPrefix string `arg:"" optional:"" help:"Project pid or prefix; defaults to active."`
}

func (c *ProjectRestartCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	pid, err := resolveProjectTarget(c.IDOrPrefix)
	if err != nil {
		return err
	}
	cname := "glovebox-agent-" + pid
	if err := hostDocker.RestartContainer(context.Background(), cname); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Restarted %s.\n", cname)
	return nil
}

// --- gbx rebuild [--all] ---

type ProjectRebuildCmd struct {
	IDOrPrefix string `arg:"" optional:"" help:"Project pid or prefix."`
	All        bool   `name:"all" help:"Rebuild every recorded project after rebuilding the image."`
	Controller bool   `name:"controller" help:"Rebuild the singleton stack-controller image and recreate its container (ignores id/--all)."`
}

func (c *ProjectRebuildCmd) Run(kctx *kong.Context) error {
	if c.Controller && (c.IDOrPrefix != "" || c.All) {
		return errors.New("gbx rebuild: --controller cannot be combined with a project id or --all")
	}
	if err := requireDocker(); err != nil {
		return err
	}
	if err := requireEnvFile(); err != nil {
		return err
	}

	if c.Controller {
		st, err := stack.FromEnv(hostDocker, hostClient)
		if err != nil {
			return err
		}
		if err := st.RebuildController(context.Background(), kctx.Stderr); err != nil {
			return err
		}
		fmt.Fprintln(kctx.Stdout, "Rebuilt and recreated the stack-controller.")
		return nil
	}

	libexec := os.Getenv("GBX_LIBEXEC")
	if err := hostDocker.BuildImage(context.Background(), agentBuildSpec(libexec)); err != nil {
		return err
	}

	var pids []string
	if c.All {
		projectsDir := filepath.Join(stateDirFromEnv(), projectsPath)
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return fmt.Errorf("can't read '%s' folder: %w", projectsDir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				pids = append(pids, e.Name())
			}
		}
	} else {
		pid, err := resolveProjectTarget(c.IDOrPrefix)
		if err != nil {
			return err
		}
		pids = append(pids, pid)
	}

	ctx := context.Background()
	base := config.GbxFromEnv().AgentImage
	for _, pid := range pids {
		stateProjDir := filepath.Join(stateDirFromEnv(), projectsPath, pid)
		wsfile := filepath.Join(stateProjDir, workspacePathFile)
		data, err := os.ReadFile(wsfile)
		if err != nil {
			fmt.Fprintf(kctx.Stderr, "Skipping %s: no workspace recorded.\n", pid)
			continue
		}
		ws := strings.TrimRight(string(data), "\r\n")

		plugins, err := plugin.List(stateProjDir)
		if err != nil {
			return fmt.Errorf("list plugins for %s: %w", pid, err)
		}
		derived := plugin.DerivedImageTag(base, pid)
		if len(plugins) > 0 {
			dfPath, werr := plugin.WriteDockerfile(stateProjDir, base, plugins)
			if werr != nil {
				return fmt.Errorf("write Dockerfile for %s: %w", pid, werr)
			}
			if berr := hostDocker.BuildImage(ctx, dockerx.BuildSpec{
				Tag:        derived,
				Dockerfile: dfPath,
				Context:    plugin.Dir(stateProjDir),
			}); berr != nil {
				return fmt.Errorf("build plugin image for %s: %w", pid, berr)
			}
		} else if hostDocker.ImageExists(ctx, derived) {
			// No plugins remain: drop the stale derived image so the project
			// cleanly reverts to the base on the next ensure. Best-effort -
			// RemoveImage is itself best-effort (a missing/locked image is not
			// fatal), so a failure here must not abort the rebuild.
			_ = hostDocker.RemoveImage(ctx, derived)
		}

		containerName := "glovebox-agent-" + pid
		if err := hostDocker.ForceRemoveContainer(ctx, containerName); err != nil {
			return fmt.Errorf("can't force remove container '%s': %w", containerName, err)
		}
		if err := ensureAgentFn(ctx, hostClient, pid, ws, libexec, stateDirFromEnv()); err != nil {
			return err
		}
		fmt.Fprintf(kctx.Stdout, "Rebuilt and recreated glovebox-agent-%s.\n", pid)
	}
	if !c.All {
		fmt.Fprintln(kctx.Stdout, "Other projects keep running the old image until rebuilt or recreated.")
	}
	return nil
}

// --- gbx state-size ---

type ProjectStateSizeCmd struct {
	IDOrPrefix string `arg:"" optional:"" help:"Project pid or prefix; defaults to active."`
}

func (c *ProjectStateSizeCmd) Run(kctx *kong.Context) error {
	pid, err := resolveProjectTarget(c.IDOrPrefix)
	if err != nil {
		return err
	}
	projDir := filepath.Join(stateDirFromEnv(), projectsPath, pid)
	fmt.Fprintf(kctx.Stdout, "PROJECT %s\n", pid)
	fmt.Fprintf(kctx.Stdout, "%-12s %s\n", "DIR", "SIZE")
	for _, d := range []string{"claude", "codex", "opencode", "pi", "gemini", "aider", "hermes"} {
		path := filepath.Join(projDir, d)
		if _, err := os.Stat(path); err == nil {
			size := duHuman(path)
			fmt.Fprintf(kctx.Stdout, "%-12s %s\n", d, size)
		}
	}
	fmt.Fprintln(kctx.Stdout, "\nSHARED")
	fmt.Fprintf(kctx.Stdout, "%-12s %s\n", "DIR", "SIZE")
	for _, d := range []string{"npm", "uv-tools", "bin", "cache", "shell-history"} {
		path := filepath.Join(stateDirFromEnv(), "shared", d)
		if _, err := os.Stat(path); err == nil {
			size := duHuman(path)
			fmt.Fprintf(kctx.Stdout, "%-12s %s\n", d, size)
		}
	}
	return nil
}

// duHuman shells out to `du -sh` so the output keeps the suffix-suffixed
// human format users expect (e.g. "120M"). The Go stdlib has no
// human-readable size helper that matches; shelling out keeps the format
// byte-compatible with the bash version.
func duHuman(path string) string {
	out, err := exec.CommandContext(context.Background(), "du", "-sh", path).Output()
	if err != nil {
		return "?"
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return "?"
	}
	return parts[0]
}

// agentBuildSpec is the canonical dockerx.BuildSpec for the shared agent
// image. Used by both `project rebuild` (always builds) and ensureAgentImage
// (builds only when missing). The tag comes from gbxconfig so tests can
// point at a throwaway image without untagging the operator's real one.
func agentBuildSpec(libexec string) dockerx.BuildSpec {
	return dockerx.BuildSpec{
		Tag:        config.GbxFromEnv().AgentImage,
		Dockerfile: filepath.Join(libexec, "docker", "Dockerfile"),
		Context:    libexec,
		Args: map[string]string{
			"HOST_UID": strconv.Itoa(agent.HostUID()),
			"HOST_GID": strconv.Itoa(agent.HostGID()),
		},
	}
}

// ensureAgentImage builds the configured agent image if it isn't present
// locally. This is what makes `gbx new` work on a fresh install without
// requiring the user to know about `gbx rebuild --all` first. The
// image-inspect probe is sub-millisecond when the image exists, so adding
// this to every agent-ensure path is essentially free.
func ensureAgentImage(libexec string) error {
	ctx := context.Background()
	img := config.GbxFromEnv().AgentImage
	if hostDocker.ImageExists(ctx, img) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "Building %s (one-time, ~5 min)...\n", img)
	return hostDocker.BuildImage(ctx, agentBuildSpec(libexec))
}
