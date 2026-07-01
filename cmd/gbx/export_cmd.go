package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/convexport"
)

// ExportConversationsCmd copies in-sandbox agent conversation logs out to the
// host locations that desktop viewers (AgentsView etc.) scan, re-tagged with
// glovebox provenance so projects don't collide under the sandbox's shared
// /workspace cwd.
type ExportConversationsCmd struct {
	Harness string `name:"harness" help:"Limit to one harness (claude, codex, gemini, opencode, aider, pi, hermes). Default: all."`
	All     bool   `name:"all" help:"Export every registered project (default: the active/-p project)."`
	Dest    string `name:"dest" type:"path" help:"Override the destination root (requires --harness; default is the harness's native dir, e.g. ~/.claude)."`
}

func (c *ExportConversationsCmd) Run(kctx *kong.Context) error {
	if c.All && os.Getenv(config.EnvOverridePID) != "" {
		return errors.New("gbx export-conversations: --all conflicts with -p <pid>")
	}
	if c.Dest != "" && c.Harness == "" {
		return errors.New("--dest requires --harness (each harness has its own destination root)")
	}
	if c.Harness != "" && !slices.Contains(agent.Names, c.Harness) {
		return fmt.Errorf("unknown harness %q; valid: %s", c.Harness, strings.Join(agent.Names, ", "))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	stateDir := config.GbxFromEnv().StateDir

	var pids []string
	if c.All {
		entries, err := os.ReadDir(filepath.Join(stateDir, config.ProjectsPath))
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(kctx.Stdout, "No projects to export.")
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
			fmt.Fprintln(kctx.Stdout, "No projects to export.")
			return nil
		}
	} else {
		pid, err := targetPID()
		if err != nil {
			return err
		}
		pids = []string{pid}
	}

	files := 0
	skipped := map[string]bool{}
	for _, pid := range pids {
		results, err := convexport.ExportProject(stateDir, home, pid, c.Harness, c.Dest)
		if err != nil {
			return err
		}
		for _, r := range results {
			switch r.Status {
			case convexport.StatusExported:
				files += r.Files
				fmt.Fprintf(kctx.Stdout, "%s  %-9s %d session(s) → %s\n",
					pid, r.Harness, r.Files, filepath.Join(r.DestDir, "projects"))
			case convexport.StatusUnsupported:
				skipped[r.Harness] = true
			}
		}
	}

	if files == 0 {
		fmt.Fprintln(kctx.Stdout, "No conversations found to export.")
	} else {
		fmt.Fprintf(kctx.Stdout, "\nExported %d session file(s).\n", files)
	}
	if len(skipped) > 0 {
		names := make([]string, 0, len(skipped))
		for n := range skipped {
			names = append(names, n)
		}
		slices.Sort(names)
		fmt.Fprintf(kctx.Stdout, "Not yet supported (scaffolded): %s\n", strings.Join(names, ", "))
	}
	return nil
}
