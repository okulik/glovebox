package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/willabides/kongplete"

	"github.com/okulik/glovebox"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/project"
)

// resolveLibexec defaults GBX_LIBEXEC to the parent of the binary's directory
// so a dev clone like `<repo>/bin/gbx` resolves to `<repo>`. If the env var
// is already set, it is preserved. The Homebrew formula sets GBX_LIBEXEC
// explicitly via bin.write_env_script and that value wins.
func resolveLibexec() {
	if os.Getenv(config.EnvLibexec) != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	os.Setenv(config.EnvLibexec, filepath.Dir(filepath.Dir(resolved)))
}

//nolint:govet // fieldalignment: Kong-driven layout; readability over packing.
type CLI struct {
	Ls                  ProjectLsCmd                 `cmd:"" help:"List projects (* marks default)."`
	Up                  UpCmd                        `cmd:"" help:"Bring the singleton egress-proxy + controller stack up (idempotent)."`
	Stack               StackCmd                     `cmd:"" help:"Per-project dev-stack management."`
	StateSize           ProjectStateSizeCmd          `cmd:"" name:"state-size" help:"Disk usage of one project + shared caches."`
	Logs                LogsCmd                      `cmd:"" help:"Tail a stack component log: proxy (egress access) or controller (HTTP server)."`
	PFlag               string                       `short:"p" name:"pid" placeholder:"PID" help:"Pid prefix; overrides default project for this call."`
	Update              UpdateCmd                    `cmd:"" help:"Update an agent inside the active project's container."`
	Start               ProjectStartCmd              `cmd:"" help:"Start the project's agent."`
	Stop                ProjectStopCmd               `cmd:"" help:"Stop the project's agent."`
	Use                 ProjectUseCmd                `cmd:"" help:"Set the default project."`
	Restart             ProjectRestartCmd            `cmd:"" help:"Restart the project's agent."`
	New                 ProjectNewCmd                `cmd:"" name:"new" help:"Register a project from a workspace path."`
	Allow               AllowCmd                     `cmd:"" help:"Append a domain to the egress allowlist."`
	Mount               ProjectMountCmd              `cmd:"" help:"Manage per-project extra bind mounts."`
	Plugin              PluginCmd                    `cmd:"" help:"Manage per-project Dockerfile plugins (add/edit/ls/rm)."`
	Rebuild             ProjectRebuildCmd            `cmd:"" help:"Rebuild the shared agent image and recreate the project's agent (or the stack-controller with --controller)."`
	Sync                SyncCmd                      `cmd:"" help:"Reconcile managed agent state from current defaults (no container recreate)."`
	Run                 RunCmd                       `cmd:"" help:"Run a command in the active project's agent (or drop into a shell)."`
	Rm                  ProjectRmCmd                 `cmd:"" help:"Stop and remove a project's agent."`
	ExportConversations ExportConversationsCmd       `cmd:"" name:"export-conversations" help:"Surface in-sandbox agent conversation logs on the host for AgentsView etc. (symlinks; --copy snapshots)."`
	Version             kong.VersionFlag             `help:"Print the gbx version and exit."`
	InstallCompletions  kongplete.InstallCompletions `cmd:"" name:"install-completions" help:"Emit a shell completion script (bash, zsh, fish). Redirect to your shell's completion file."`
}

func main() {
	resolveLibexec()

	// Bash/zsh/fish completion: kongplete reads COMP_LINE inside parser.Parse
	// and exits, so route straight to Kong with no rewriting.
	if os.Getenv("COMP_LINE") != "" {
		runKong(os.Args[1:])
		return
	}

	// Bare invocation and explicit `help` both print the multi-section usage.
	if len(os.Args) == 1 || os.Args[1] == "help" {
		printUsage()
		return
	}

	// `gbx mount` with no subcommand, or `gbx mount --help` / `-h`.
	if os.Args[1] == "mount" {
		if len(os.Args) == 2 || os.Args[2] == "--help" || os.Args[2] == "-h" {
			printMountUsage()
			return
		}
	}

	// `gbx stack` with no subcommand, or `gbx stack --help` / `-h`.
	if os.Args[1] == "stack" {
		if len(os.Args) == 2 || os.Args[2] == "--help" || os.Args[2] == "-h" {
			printStackUsage()
			return
		}
	}

	// `gbx plugin` with no subcommand, or `gbx plugin --help` / `-h`.
	if os.Args[1] == "plugin" {
		if len(os.Args) == 2 || os.Args[2] == "--help" || os.Args[2] == "-h" {
			printPluginUsage()
			return
		}
	}

	// Resolve `-p <pid-prefix>` once so the prefix is checked before any
	// subcommand work; the resolved full pid is exposed to downstream commands
	// via GBX_OVERRIDE_PID. The same flag is also wired into Kong (cli.PFlag)
	// for `<cmd> -p <pid>` ordering; only one path fires per invocation.
	args := os.Args[1:]
	if len(args) >= 2 && (args[0] == "-p" || args[0] == "--pid") {
		full, err := project.Resolve(config.GbxFromEnv().StateDir, args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "gbx:", err)
			os.Exit(1)
		}
		os.Setenv(config.EnvOverridePID, full)
		args = args[2:]
		if len(args) == 0 {
			printUsage()
			return
		}
	}

	// Reject unknown top-level commands with a one-line error rather than
	// Kong's verbose "unexpected argument" message.
	if !isKnownTopLevel(args[0]) {
		fmt.Fprintf(os.Stderr, "gbx: Unknown command: %s (try `gbx help`)\n", args[0])
		os.Exit(1)
	}
	runKong(args)
}

// runKong wires up Kong + kongplete, parses argv, and dispatches.
func runKong(args []string) {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("gbx"),
		kong.Description("glovebox host CLI"),
		kong.Vars{"version": glovebox.Version()},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gbx:", err)
		os.Exit(1)
	}
	// kongplete.Complete short-circuits parser.Parse if the shell is asking
	// for completion via COMP_LINE/etc.
	kongplete.Complete(parser)
	ctx, err := parser.Parse(args)
	if err != nil {
		parser.FatalIfErrorf(err)
	}
	if cli.PFlag != "" {
		full, perr := project.Resolve(config.GbxFromEnv().StateDir, cli.PFlag)
		if perr != nil {
			fmt.Fprintln(os.Stderr, "gbx:", perr)
			os.Exit(1)
		}
		os.Setenv(config.EnvOverridePID, full)
	}
	if err := ctx.Run(); err != nil {
		ctx.FatalIfErrorf(err)
	}
}
