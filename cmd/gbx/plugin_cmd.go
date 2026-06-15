package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/okulik/glovebox/internal/plugin"
)

const (
	projectsPath = "projects"
)

// PluginCmd is the `gbx plugin ...` group. All subcommands target the pid
// resolved by targetPID() (`-p <id>` if set, else the active project).
//
//nolint:govet // fieldalignment: Kong-driven layout; readability over packing.
type PluginCmd struct {
	Ls   PluginLsCmd   `cmd:"" help:"List the project's plugins."`
	Add  PluginAddCmd  `cmd:"" help:"Author a new plugin fragment in $EDITOR."`
	Edit PluginEditCmd `cmd:"" help:"Edit an existing plugin fragment in $EDITOR."`
	Rm   PluginRmCmd   `cmd:"" help:"Remove a plugin fragment."`
}

// launchEditor opens path in the user's editor and blocks until it exits.
// $VISUAL wins over $EDITOR; neither set is a user-actionable error. Tests
// swap this var to avoid spawning a real editor.
var launchEditor = func(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return errors.New("No editor configured. Set $EDITOR, e.g. `export EDITOR=vim`.")
	}
	// Allow editors carrying flags, e.g. EDITOR="code --wait".
	fields := strings.Fields(editor)
	//nolint:gosec // G204: editor comes from the user's own environment.
	cmd := exec.Command(fields[0], append(fields[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// pluginProjectDir resolves the target project's state dir, creating it if
// necessary (plugin add may run before the first agent is created).
func pluginProjectDir() (string, error) {
	pid, err := targetPID()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(stateDirFromEnv(), projectsPath, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create state dir for %s: %w", pid, err)
	}
	return dir, nil
}

// --- gbx plugin add ---

type PluginAddCmd struct{}

func (c *PluginAddCmd) Run(kctx *kong.Context) error {
	projDir, err := pluginProjectDir()
	if err != nil {
		return err
	}
	pid := filepath.Base(projDir)
	pluginsDir := plugin.Dir(projDir)
	err = os.MkdirAll(pluginsDir, 0o755)
	if err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}
	// Editor scratch file lives in the plugins dir (so the final rename is
	// same-filesystem) but is hidden so List ignores it.
	var draft *os.File
	draft, err = os.CreateTemp(pluginsDir, ".draft-*.dockerfile")
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	draftName := draft.Name()
	_, err = draft.WriteString(plugin.Template(pid))
	if err != nil {
		draft.Close()
		_ = os.Remove(draftName)
		return fmt.Errorf("seed draft: %w", err)
	}
	err = draft.Close()
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	err = launchEditor(draftName)
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	var content []byte
	content, err = os.ReadFile(draftName)
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	var id string
	id, err = plugin.Store(projDir, string(content), time.Now())
	if err != nil {
		// Keep the draft so the user doesn't lose their work.
		fmt.Fprintf(kctx.Stderr, "Draft kept at %s\n", draftName)
		return err
	}
	_ = os.Remove(draftName)
	fmt.Fprintf(kctx.Stdout, "Stored plugin %s. Run `gbx rebuild` to apply.\n", id)
	return nil
}

// --- gbx plugin edit <id> ---

type PluginEditCmd struct {
	IDOrPrefix string `arg:"" help:"Plugin id or prefix."`
}

func (c *PluginEditCmd) Run(kctx *kong.Context) error {
	projDir, err := pluginProjectDir()
	if err != nil {
		return err
	}
	var p plugin.Plugin
	p, err = plugin.Find(projDir, c.IDOrPrefix)
	if err != nil {
		return err
	}
	// Edit a copy so a botched edit can't corrupt the stored fragment.
	var draft *os.File
	draft, err = os.CreateTemp(plugin.Dir(projDir), ".draft-*.dockerfile")
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	draftName := draft.Name()
	_, err = draft.WriteString(p.Content)
	if err != nil {
		draft.Close()
		_ = os.Remove(draftName)
		return fmt.Errorf("seed draft: %w", err)
	}
	err = draft.Close()
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	err = launchEditor(draftName)
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	var content []byte
	content, err = os.ReadFile(draftName)
	if err != nil {
		_ = os.Remove(draftName)
		return err
	}
	err = plugin.Overwrite(p, string(content))
	if err != nil {
		fmt.Fprintf(kctx.Stderr, "Draft kept at %s\n", draftName)
		return err
	}
	_ = os.Remove(draftName)
	fmt.Fprintf(kctx.Stdout, "Updated plugin %s. Run `gbx rebuild` to apply.\n", p.ID)
	return nil
}

// --- gbx plugin ls ---

type PluginLsCmd struct {
	All bool `name:"all" help:"List plugins for every project."`
}

func (c *PluginLsCmd) Run(kctx *kong.Context) error {
	const tsLayout = "2006-01-02 15:04"
	if c.All {
		projectsRoot := filepath.Join(stateDirFromEnv(), projectsPath)
		entries, err := os.ReadDir(projectsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read projects dir: %w", err)
		}
		fmt.Fprintf(kctx.Stdout, "%-12s  %-8s  %-16s  %s\n", "PROJECT", "PLUGIN", "MODIFIED", "DESCRIPTION")
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pid := e.Name()
			plugins, err := plugin.List(filepath.Join(projectsRoot, pid))
			if err != nil {
				return err
			}
			for _, p := range plugins {
				fmt.Fprintf(kctx.Stdout, "%-12s  %-8s  %-16s  %s\n",
					pid, p.ID, p.ModTime.Format(tsLayout), p.Description)
			}
		}
		return nil
	}

	projDir, err := pluginProjectDir()
	if err != nil {
		return err
	}
	plugins, err := plugin.List(projDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "%-8s  %-16s  %s\n", "PLUGIN", "MODIFIED", "DESCRIPTION")
	for _, p := range plugins {
		fmt.Fprintf(kctx.Stdout, "%-8s  %-16s  %s\n", p.ID, p.ModTime.Format(tsLayout), p.Description)
	}
	return nil
}

// --- gbx plugin rm <id> ---

type PluginRmCmd struct {
	IDOrPrefix string `arg:"" help:"Plugin id or prefix."`
	Yes        bool   `short:"y" name:"yes" help:"Skip the y/N prompt."`
}

func (c *PluginRmCmd) Run(kctx *kong.Context) error {
	projDir, err := pluginProjectDir()
	if err != nil {
		return err
	}
	p, err := plugin.Find(projDir, c.IDOrPrefix)
	if err != nil {
		return err
	}
	if !c.Yes && !confirmYN(kctx.Stderr, fmt.Sprintf("Remove plugin %s (%s)? [y/N] ", p.ID, p.Description)) {
		return errors.New("Aborted.")
	}
	if err := plugin.Remove(p); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "Removed plugin %s. Run `gbx rebuild` to apply.\n", p.ID)
	return nil
}
