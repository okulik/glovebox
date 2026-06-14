package main

import (
	"fmt"
)

// printUsage prints the top-level help. Used by `gbx help` and by the no-args
// invocation.
func printUsage() {
	cfg := configDirFromEnv()
	fmt.Printf(`Usage: gbx [-p <id>] <command> [args]

Projects (target the default project unless -p <id> is given):
  new <host-path>                    Register a project from a workspace path.
                                     Creates the per-project agent; sets it as
                                     default if none yet.
  use <id-or-prefix>                 Set the default project.
  ls [-v / --verbose]                List projects (* marks default). -v also
                                     lists each project's agent and stack
                                     containers (name, status, labels).
  rm <id> [--delete-state] [--yes]   Stop and remove a project's agent. State
                                     dir is kept by default; pass --delete-state
                                     to wipe it too.
  start   [<id>]                     Start the project's agent.
  stop    [<id>]                     Stop the project's agent.
  restart [<id>]                     Restart the project's agent.
  rebuild [<id>] [--all]             Rebuild shared agent image and recreate
                                     the project's agent (or all with --all).
  rebuild --controller               Rebuild the stack-controller image and
                                     recreate its container.
  sync [--all]                       Reconcile managed agent state from current defaults
  state-size [<id>]                  Disk usage of one project + shared caches.
  mount add <host>[:<container>][:rw|ro]
                                     Append an extra bind mount to the project.
  mount rm <host-or-container>       Remove a previously added mount.
  mount ls                           List the project's extra mounts.
  mount apply                        Recreate the agent container with the
                                     current mount set.

Agents:
  run                                Drop into a bash shell inside the agent.
  run [--] <cmd...>                  Run a one-shot command.
  run <agent> [args...]              Launch one of: claude, codex, opencode,
                                     pi, gemini, aider, hermes.
  update <agent>                     Update an agent inside the container.
  logs [proxy]                       Tail the shared egress-proxy access log.

Global:
  up                                 Bring the singleton egress-proxy +
                                     controller stack up (idempotent).
  allow <domain>                     Add to the shared egress allowlist.
  stack <subcommand>                 Manage per-project dev stacks.
  help                               Show this message.
  -p <id-or-prefix> <cmd>            Override the default project for one call.

Config dir: %s
`, cfg)
}

// printStackUsage prints the `gbx stack` no-arg help.
func printStackUsage() {
	fmt.Println(`gbx stack - manage per-project dev stacks

Commands (target the default project unless -p <id> is given):
  [-p <id>] stack diff                    Show live vs proposed (from controller)
  [-p <id>] stack apply [--dry-run] [-y]  Apply the stored proposal
  [-p <id>] stack down                    Stop services, keep volumes
  [-p <id>] stack destroy [-y]            Stop + remove services + volumes
  [-p <id>] stack status                  Show service health
            stack ls                      List all projects with stacks
  [-p <id>] stack logs <svc> [--follow]   Stream service logs
            stack image-allow <registry>  Extend image-allowlist.txt`)
}

// isKnownTopLevel reports whether the first positional argv is one of the
// Kong-registered top-level subcommands or a flag.
func isKnownTopLevel(s string) bool {
	if len(s) > 0 && s[0] == '-' {
		return true // a flag - let Kong handle it
	}
	switch s {
	case "new", "use", "ls", "rm",
		"start", "stop", "restart", "rebuild", "sync",
		"state-size", "mount",
		"stack", "up",
		"run", "logs", "allow", "update",
		"install-completions",
		"help":
		return true
	}
	return false
}
