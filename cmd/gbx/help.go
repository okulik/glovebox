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
  plugin add | edit <id> | ls | rm <id>
                                     Manage per-project Dockerfile plugins
                                     (layered on the base image; apply with
                                     'gbx rebuild').
  stack diff                         Show live vs proposed manifest.
  stack apply [--dry-run] [-y]       Apply the stored proposal.
  stack down                         Stop services, keep volumes.
  stack destroy [-y]                 Stop + remove services + volumes.
  stack status                       Show service health.
  stack ls                           List all projects with stacks.
  stack logs <svc> [--follow]        Stream service logs.
  stack image-allow <registry>       Extend the image allowlist.


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
  help                               Show this message.

Config dir: %s
`, cfg)
}

// printMountUsage prints the `gbx mount --help` / `-h` help.
func printMountUsage() {
	fmt.Println(`gbx mount - manage per-project extra bind mounts

All subcommands target the default project unless -p <id> is given.
Changes take effect on the next 'gbx mount apply' or 'gbx rebuild'.

  add <host>[:<container>][:rw|ro]

      Append a bind mount to the project's stored mount set.
      <host> is symlink-resolved before storing so the on-disk record
      matches what Docker actually mounts.
      <container> defaults to /mnt/<basename of host>.
      mode defaults to rw.
      Container paths already claimed by the runtime
      (/workspace, /home/gbx/.claude, /home/gbx/.npm, …) are rejected
      to prevent shadowing agent state.

  rm <host-or-container>

      Remove a previously added mount, matched against either the host
      path or the container path. The unresolved host path is also
      accepted (the match tries both forms).

  ls

      Print the current mount set, one host:container:mode per line.

  apply

      Force-recreate the agent container so the current mount set takes
      effect immediately. Equivalent to 'gbx rebuild' but skips the
      image rebuild step.`)
}

// printStackUsage prints the `gbx stack` / `gbx stack --help` help.
func printStackUsage() {
	fmt.Println(`gbx stack - manage per-project dev stacks

All subcommands target the default project unless -p <id> is given.

  diff                           Show live manifest vs stored proposal.
                                 Outputs a unified diff (--- live / +++ proposed)
                                 when a proposal is pending.

  apply [--dry-run] [-y]         Apply the stored proposal via the controller.
                                 --dry-run prints the proposed manifest without
                                 applying it. -y skips the confirmation prompt.

  down                           Stop all services, keep their volumes.

  destroy [-y]                   Stop services and remove all volumes.
                                 Irreversible. -y skips the confirmation prompt.

  status                         Show health of each service in the stack.

  ls                             List every project that has an active stack.

  logs <service> [--follow]      Stream logs for a named service.
                                 --follow tails live output.

  image-allow <registry>         Append a registry to docker/image-allowlist.txt.
                                 Restart the controller to apply:
                                   gbx restart controller`)
}

// printPluginUsage prints the `gbx plugin` / `gbx plugin --help` help.
func printPluginUsage() {
	fmt.Println(`gbx plugin - manage per-project Dockerfile plugins

Plugins are Dockerfile fragments layered on top of the base agent image.
All subcommands target the default project unless -p <id> is given.
Changes take effect on the next 'gbx rebuild'.

  add                 Open $EDITOR on a new fragment seeded with instructions.
                      The fragment must carry a '# gbx:description: <text>'
                      line. Stored under
                      state/projects/<pid>/plugins/<pluginid>.

  edit <id>           Open $EDITOR on an existing fragment (id prefix ok).

  ls [--all]          List the project's plugins (id, modified, description).
                      --all lists every project's plugins.

  rm <id> [-y]        Remove a fragment (id prefix ok). -y skips the prompt.

A project with no plugins runs the shared base image. A project with one or
more plugins runs a derived image 'glovebox-agent-<pid>:local' built by
'gbx rebuild'.`)
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
		"state-size", "mount", "plugin",
		"stack", "up",
		"run", "logs", "allow", "update",
		"install-completions",
		"help":
		return true
	}
	return false
}
