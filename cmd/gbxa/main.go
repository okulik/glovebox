package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/okulik/glovebox"
)

// helpWrapWidth caps help-text wrapping so the description stays readable on
// wide terminals instead of stretching to the full window width.
const helpWrapWidth = 80

type CLI struct {
	Version kong.VersionFlag `help:"Print the gbxa version and exit."`
}

// appDescription is the gbxa help blurb. gbxa is a multi-call binary,
// normally invoked under one of the agent names symlinked to it (see the
// Dockerfile) rather than as "gbxa"; listing those names means a bare `gbxa`
// explains what the binary is actually for.
func appDescription() string {
	return fmt.Sprintf(
		"glovebox agent dispatcher (in-container).\n\n"+
			"Multi-call binary: invoke it under an agent name to launch that agent, "+
			"lazily installing it and exec'ing it in /workspace. Wrapped agents: %s. "+
			"The gbx-stack name runs the in-container stack CLI.",
		strings.Join(agentNames(), ", "),
	)
}

func main() {
	name := filepath.Base(os.Args[0])
	switch name {
	case "gbxa":
		// fall through to the Kong tree
	case "gbx-stack":
		if err := dispatchAgentStack(); err != nil {
			fmt.Fprintln(os.Stderr, "gbx-stack:", err)
			os.Exit(1)
		}
		return
	default:
		if err := dispatch(); err != nil {
			fmt.Fprintln(os.Stderr, "gbxa:", err)
			os.Exit(1)
		}
		return
	}

	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("gbxa"),
		kong.Description(appDescription()),
		kong.Vars{"version": glovebox.Version()},
		// Cap help wrapping at a readable width instead of letting the
		// description sprawl across a wide terminal.
		kong.ConfigureHelp(kong.HelpOptions{WrapUpperBound: helpWrapWidth}),
	)
	if ctx.Command() == "" {
		_ = ctx.PrintUsage(false)
		return
	}
	if err := ctx.Run(); err != nil {
		ctx.FatalIfErrorf(err)
	}
}

// defaultToHelp maps a bare argv (no command) to ["--help"] so a wrapper
// invoked with no arguments prints help instead of erroring with "expected
// one of ...". Kong returns a nil context for an empty parse of a tree with
// required subcommands, so we can't use the gbxa-style PrintUsage path here.
func defaultToHelp(argv []string) []string {
	if len(argv) == 0 {
		return []string{"--help"}
	}
	return argv
}

// parseAndRun is a thin wrapper around Kong used by dispatch helpers (e.g.
// dispatchAgentStack) that need an isolated parser without the gbxa top-level
// commands. A bare invocation prints help and exits 0.
func parseAndRun(cli any, argv []string, name string) error {
	parser, err := kong.New(cli, kong.Name(name),
		kong.ConfigureHelp(kong.HelpOptions{WrapUpperBound: helpWrapWidth}),
	)
	if err != nil {
		return err
	}
	kctx, err := parser.Parse(defaultToHelp(argv))
	if err != nil {
		return err
	}
	return kctx.Run()
}
