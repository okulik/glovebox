package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/project"
	"github.com/okulik/glovebox/internal/stack"
	"github.com/okulik/glovebox/internal/state"
)

// hostDocker is the dockerx.Host instance used by every command in cmd/gbx.
var hostDocker dockerx.HostClient

// hostClient is the API-level companion to hostDocker for read/inspect ops.
var hostClient dockerx.ControllerClient

// confirmYN prompts on stderr and reads a y/N answer from stdin, returning
// true only on an explicit "y".
func confirmYN(stderr io.Writer, prompt string) bool {
	fmt.Fprint(stderr, prompt)
	var ans string
	if _, err := fmt.Fscanln(os.Stdin, &ans); err != nil {
		return false
	}
	return strings.EqualFold(ans, "y")
}

// requireDocker returns error if the docker daemon is unreachable.
func requireDocker() error {
	if hostDocker == nil {
		h, err := dockerx.NewHostClient()
		if err != nil {
			return fmt.Errorf("initialize docker client: %w", err)
		}
		hostDocker = h
	}
	if hostClient == nil {
		c, err := dockerx.NewControllerClientFromEnv()
		if err != nil {
			return fmt.Errorf("initialize docker API client: %w", err)
		}
		hostClient = c
	}
	if err := hostDocker.DaemonReachable(context.Background()); err != nil {
		return errors.New("Docker is not reachable. Start your Docker runtime first (OrbStack, Docker Desktop, Colima, Rancher Desktop, …).")
	}
	return nil
}

// projectClient returns a dockerx.Client suitable for read-only project
// status queries.
func projectClient() dockerx.ControllerClient {
	if hostClient != nil {
		return hostClient
	}
	c, err := dockerx.NewControllerClientFromEnv()
	if err != nil {
		return nil
	}
	hostClient = c
	return c
}

// requireEnvFile errors if ${GBX_CONFIG_DIR}/.env is missing.
func requireEnvFile() error {
	envFile := filepath.Join(config.GbxFromEnv().ConfigDir, ".env")
	if _, err := os.Stat(envFile); err != nil {
		return fmt.Errorf(".env not found at %s - run `gbx new <path>` to bootstrap.", envFile)
	}
	return nil
}

// ensureStackUp brings the singleton stack up if egress-proxy isn't running.
func ensureStackUp(ctx context.Context) error {
	st, err := stack.FromEnv(hostDocker, hostClient)
	if err != nil {
		return err
	}
	running, err := st.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("can't detect if proxy is running: %w", err)
	}
	if running {
		return nil
	}
	return st.Up(ctx, os.Stderr)
}

// resolveProjectTarget returns the pid for `q`, or the active pid if `q` is
// empty.
func resolveProjectTarget(q string) (string, error) {
	if q == "" {
		pid, err := state.ActivePID(config.GbxFromEnv().ConfigDir)
		if err != nil {
			return "", err
		}
		if pid == "" {
			return "", errors.New("No default project. Run 'gbx use <id>' or pass an explicit id.")
		}
		return pid, nil
	}
	return project.Resolve(config.GbxFromEnv().StateDir, q)
}

// targetPID resolves the per-invocation pid: GBX_OVERRIDE_PID > active project.
func targetPID() (string, error) {
	if p := os.Getenv(config.EnvOverridePID); p != "" {
		return p, nil
	}
	if err := state.RequireSomeProject(config.GbxFromEnv().StateDir); err != nil {
		return "", err
	}
	pid, err := state.ActivePID(config.GbxFromEnv().ConfigDir)
	if err != nil {
		return "", err
	}
	if pid == "" {
		return "", errors.New("No default project. Run 'gbx use <id>' or pass -p <id>.")
	}
	return pid, nil
}

// ensureTargetAgent resolves the target pid, ensures the agent container
// exists and is running, and returns the container name.
func ensureTargetAgent(ctx context.Context) (string, error) {
	pid, err := targetPID()
	if err != nil {
		return "", err
	}
	wsfile := filepath.Join(config.GbxFromEnv().StateDir, config.ProjectsPath, pid, config.WorkspacePath)
	wsData, err := os.ReadFile(wsfile)
	if err != nil {
		return "", fmt.Errorf("Project %s has no recorded workspace. Run 'gbx new <path>' for it first.", pid)
	}
	ws := string(wsData)
	for len(ws) > 0 && (ws[len(ws)-1] == '\n' || ws[len(ws)-1] == '\r') {
		ws = ws[:len(ws)-1]
	}
	libexec := os.Getenv(config.EnvLibexec)
	if err := ensureAgentFn(ctx, hostClient, pid, ws, libexec, config.GbxFromEnv().StateDir); err != nil {
		return "", err
	}
	return config.ContainerAgentPrefix + pid, nil
}
