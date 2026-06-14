# gbx 1 "2026" "glovebox" "User Commands"

## NAME

gbx - isolated multi-agent AI coding harness

## SYNOPSIS

`gbx` [**-p** *pid*] *command* [*args...*]

## DESCRIPTION

**gbx** is the host-side CLI for glovebox, a per-project sandbox that runs AI
coding agents (Claude Code, Codex, OpenCode, Gemini, Aider, Hermes, Pi) inside
Docker containers with egress restricted by a Squid proxy. State is bind-mounted
from the host so agent installs persist across container restarts.

Each *project* is identified by a 12-character pid derived from the workspace
path. The default project is recorded at *$GBX_CONFIG_DIR/active-project* and
applies to every command that doesn't pass **-p**.

## GLOBAL FLAGS

`-p`, `--pid` *PID*
:   Override the default project for this invocation. *PID* is a prefix; gbx
    resolves it to the unique matching project and refuses ambiguous prefixes.

`--help`
:   Print Kong-style help for the current command.

`--version`
:   Print the gbx version and exit.

## PROJECT MANAGEMENT

`gbx new` *path*
:   Register *path* as a project. Creates the per-project agent container and,
    if no default is set, makes the new project the default.

`gbx use` *id-or-prefix*
:   Switch the default project pointer.

`gbx ls`
:   List projects; the default is marked with `*`.

`gbx rm` *id-or-prefix* [`--delete-state`] [`-y` / `--yes`]
:   Stop and remove the project's agent container. The bind-mounted state
    directory is preserved by default so a future `gbx new` on the same
    workspace path picks up where the agent left off; pass `--delete-state`
    to wipe it.

`gbx rm --all` [`--delete-state`] [`-y` / `--yes`]
:   Remove every registered project. Prompts once with the full pid list
    unless `-y`/`--yes` is given. State dirs are kept unless
    `--delete-state` is passed.

`gbx start` | `stop` | `restart` [*id*]
:   Per-project agent lifecycle.

`gbx rebuild` [*id*] [`--all`] [`--controller`]
:   Rebuild the shared `glovebox-agent:local` image and recreate the project's
    agent container. With `--all`, every recorded project is recreated. With
    `--controller`, rebuild the singleton `glovebox-stack-controller:local`
    image from source and recreate its container instead — use this to pick up
    new controller code (e.g. added API routes) that `gbx up` would skip
    because the image already exists. `--controller` cannot be combined with an
    *id* or `--all`.

`gbx state-size` [*id*]
:   Show disk usage of one project's state directories plus the shared caches.

`gbx mount add` *host*[`:`*container*][`:rw`|`:ro`]
:   Append a host bind mount to the project's mount set. A bare host path
    mounts at `/mnt/<basename>` read-write. The set is persisted at
    `$GBX_CONFIG_DIR/state/projects/<pid>/mounts.txt` and applied on the next
    agent (re)create. Use `gbx mount apply` to push changes into a
    running container.

`gbx mount rm` *host-or-container*
:   Remove a previously added mount, matched against either the host or
    container path.

`gbx mount ls`
:   Print the project's extra mounts, one `host:container:mode` per line.

`gbx mount apply`
:   Force-remove the agent container and recreate it so the current mount
    set takes effect.

## AGENT COMMANDS

`gbx run` [`--`] [*cmd...*]
:   With no positional command, drop into a login bash shell inside the active
    project's agent container. With arguments, exec them in the agent.

`gbx run` *agent* [*args...*]
:   Launch one of the bundled agents (`claude`, `codex`, `opencode`, `pi`,
    `gemini`, `aider`, `hermes`) inside the active project's container. The
    agent's own help and flags flow through unchanged.

`gbx update` *agent*
:   Reinstall the given agent inside the active project's container at its
    latest published version.

`gbx logs` [`proxy`]
:   Tail the shared egress-proxy (Squid) access log. The optional positional
    is kept for forward-compatibility; `proxy` is the only target today.

## STACK MANAGEMENT

`gbx` [`-p` *id*] `stack apply` [`--dry-run`] [`-y`]
:   Apply the controller's stored proposed manifest for the target project.
    `--dry-run` prints the proposal without applying; `apply` prompts `[y/N]`
    unless `-y` is given. Select the project with the global `-p`/`--pid` flag
    (before the subcommand), `GBX_PROJECT_ID`, or the active project.

`gbx` [`-p` *id*] `stack` `diff` | `down` | `destroy` | `status` | `ls` | `logs` *svc* | `image-allow` *registry*
:   See `gbx stack --help` for each subcommand. All except `ls` and
    `image-allow` select the project via `-p`/`--pid`, `GBX_PROJECT_ID`, or the
    active project; `destroy` prompts `[y/N]` unless `-y` is given.

## STACK LIFECYCLE

`gbx up`
:   Bring the singleton shared stack (egress-proxy, socket-proxy,
    stack-controller) up if it isn't already. Idempotent; on first run also
    builds the controller image and pulls the proxy images. Every other
    command that needs the stack (`gbx new`, `gbx run`, `gbx start`, …)
    calls this automatically, but running it explicitly is useful as a
    one-shot bootstrap or operator sanity check.

## ALLOWLIST

`gbx allow` *domain*
:   Append *domain* to the shared egress allowlist and restart the Squid proxy
    so it takes effect.

## INSTALL COMPLETIONS

`gbx install-completions`
:   Emit a shell completion script for the parent shell (bash, zsh, fish).
    Redirect to your shell's completion directory; see EXAMPLES below.

## ENVIRONMENT

`GBX_LIBEXEC`
:   Path to read-only package files (docker recipes, defaults, .env.example).
    Defaults to the parent of the directory containing the gbx binary.

`GBX_CONFIG_DIR`
:   User-writable state directory. Defaults to `~/.config/glovebox`.

`GBX_OVERRIDE_PID`
:   Set by the `-p` flag for one invocation. If set externally, every gbx
    command targets that pid until the variable is cleared.

`GBX_CONTROLLER_URL`, `GBX_CONTROLLER_HOST_PORT`
:   Override how `gbx stack` reaches the controller's HTTP API (defaults to
    `http://127.0.0.1:17001`).

## FILES

`$GBX_CONFIG_DIR/.env`
:   Per-host environment (proxy ports, workspace pointer, API keys).

`$GBX_CONFIG_DIR/active-project`
:   Two-line file: the default project's pid then its workspace path.

`$GBX_CONFIG_DIR/state/projects/<pid>/`
:   Per-project state (agent home directories, workspace-path pointer).

`$GBX_CONFIG_DIR/state/shared/`
:   Caches shared across all projects (npm, uv tools, shell history).

`$GBX_CONFIG_DIR/allowlist.txt`
:   Egress allowlist consumed by the Squid proxy.

## EXAMPLES

Register a new project and drop into its shell:

    gbx new /work/my-app
    gbx run

Open an interactive Claude Code session in the default project:

    gbx claude

Run a one-shot command in another project without switching the default:

    gbx -p ab12 run -- npm test

Enable fish completion:

    gbx install-completions > ~/.config/fish/completions/gbx.fish
