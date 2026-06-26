package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
)

// SyncCmd reconciles each target project's glovebox-managed state files with
// the current shipped defaults, without recreating the container. Changes take
// effect on the agent's next session.
type SyncCmd struct {
	All bool `name:"all" help:"Sync every registered project."`
}

func (c *SyncCmd) Run(kctx *kong.Context) error {
	if c.All && os.Getenv("GBX_OVERRIDE_PID") != "" {
		return errors.New("gbx sync: --all conflicts with -p <pid>")
	}
	stateDir := config.GbxFromEnv().StateDir

	var pids []string
	if c.All {
		entries, err := os.ReadDir(filepath.Join(stateDir, config.ProjectsPath))
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(kctx.Stdout, "No projects to sync.")
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
			fmt.Fprintln(kctx.Stdout, "No projects to sync.")
			return nil
		}
	} else {
		pid, err := targetPID()
		if err != nil {
			return err
		}
		pids = []string{pid}
	}

	libexec := os.Getenv("GBX_LIBEXEC")
	if libexec == "" {
		return errors.New("GBX_LIBEXEC not set (should be set by bin/gbx)")
	}
	dockerDir := filepath.Join(libexec, "docker")

	anyChanged := false
	failed := false
	for _, pid := range pids {
		stateProjDir := filepath.Join(stateDir, config.ProjectsPath, pid)
		changed, err := agent.ReconcileState(stateProjDir, dockerDir)
		if err != nil {
			fmt.Fprintf(kctx.Stderr, "sync %s: %v\n", pid, err)
			failed = true
			continue
		}
		if len(changed) == 0 {
			fmt.Fprintf(kctx.Stdout, "%s: already up to date\n", pid)
		} else {
			anyChanged = true
			fmt.Fprintf(kctx.Stdout, "%s: updated %s\n", pid, strings.Join(changed, ", "))
		}
	}
	if anyChanged {
		fmt.Fprintln(kctx.Stdout, "Changes take effect on the agent's next session.")
	}
	if failed {
		return errors.New("gbx sync: one or more projects failed")
	}
	return nil
}
