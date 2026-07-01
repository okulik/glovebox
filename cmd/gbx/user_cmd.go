package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"golang.org/x/term"

	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/stack"
)

// interactiveStdio reports whether BOTH stdin and stdout are terminals. We
// only allocate a remote PTY when both are: if stdout is a pipe (the user
// captured it via $(...) or piped through grep), a PTY would convert the
// program's newlines to CRLF and silently corrupt the capture. This matches
// docker CLI's `-t` behavior, which drops the flag when stdout isn't a TTY.
func interactiveStdio() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// --- gbx run -- <cmd> ---

type RunCmd struct {
	Args []string `arg:"" passthrough:"" optional:"" help:"command to execute (after --)"`
}

func (c *RunCmd) Run() error {
	if err := requireDocker(); err != nil {
		return err
	}
	if err := requireEnvFile(); err != nil {
		return err
	}
	args := c.Args
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	ctx := context.Background()
	if err := ensureStackUp(ctx); err != nil {
		return err
	}
	cname, err := ensureTargetAgent(ctx)
	if err != nil {
		return err
	}
	// No positional command → drop into a login bash shell. With args, exec
	// them. TTY allocation tracks stdin's terminal-ness.
	if len(args) == 0 {
		args = []string{"bash", "-l"}
	}
	err = hostDocker.Exec(ctx, dockerx.ExecSpec{
		Container:   cname,
		Interactive: interactiveStdio(),
		User:        agent.HostUser(),
		Workdir:     agent.WorkspaceDir,
		Argv:        args,
	})
	if err != nil {
		// Both *exec.ExitError (BuildImage shells out) and the dockerx.ExitError
		// returned by the SDK-backed Exec satisfy this shape, so one check covers
		// both paths.
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		return err
	}
	return nil
}

// --- gbx logs [proxy] ---

// LogsCmd tails a singleton-stack component's logs. The optional positional
// defaults to "proxy" (the egress-proxy access log); "controller" streams the
// stack-controller's HTTP-server output.
type LogsCmd struct {
	Target string `arg:"" optional:"" default:"proxy" help:"proxy (egress access log) or controller (stack-controller HTTP server log)"`
}

func (c *LogsCmd) Run() error {
	if err := requireDocker(); err != nil {
		return err
	}
	ctx := context.Background()
	st, err := stack.FromEnv(hostDocker, hostClient)
	if err != nil {
		return err
	}
	switch c.Target {
	case "proxy":
		return st.ProxyLogs(ctx)
	case "controller":
		return st.ControllerLogs(ctx, os.Stdout)
	default:
		return errors.New("Usage: gbx logs [proxy|controller]")
	}
}

// --- gbx allow <domain> ---

type AllowCmd struct {
	Domain string `arg:"" help:"domain to add to the egress allowlist"`
}

func (c *AllowCmd) Run(kctx *kong.Context) error {
	if err := requireDocker(); err != nil {
		return err
	}
	d := c.Domain
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	allowlist := filepath.Join(config.GbxFromEnv().ConfigDir, "allowlist.txt")
	if _, err := os.Stat(allowlist); err != nil {
		return fmt.Errorf("Allowlist not found at %s - run `gbx new <path>` to bootstrap.", allowlist)
	}
	data, err := os.ReadFile(allowlist)
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == d || trimmed == "."+d {
			fmt.Fprintf(kctx.Stdout, "Already in allowlist: %s\n", d)
			return nil
		}
	}
	f, err := os.OpenFile(allowlist, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, werr := f.WriteString(d + "\n"); werr != nil {
		return werr
	}
	fmt.Fprintf(kctx.Stdout, "Appended: %s\n", d)
	ctx := context.Background()
	st, err := stack.FromEnv(hostDocker, hostClient)
	if err != nil {
		return err
	}
	if err := st.RestartProxy(ctx); err != nil {
		return err
	}
	fmt.Fprintln(kctx.Stdout, "Restarted proxy. Use `gbx logs proxy` to confirm.")
	return nil
}

// --- gbx update <agent> ---

type UpdateCmd struct {
	Agent string `arg:"" help:"agent name to update"`
}

func (c *UpdateCmd) Run() error {
	if err := requireDocker(); err != nil {
		return err
	}
	ctx := context.Background()
	if err := ensureStackUp(ctx); err != nil {
		return err
	}
	pid, err := targetPID()
	if err != nil {
		return err
	}
	cname := config.ContainerAgentPrefix + pid

	// Resolve the install argv on the host and exec it directly inside the
	// container via the SDK, which propagates the child's exit code cleanly.
	argv, err := agent.UpdateArgv(ctx, c.Agent)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "$ %s\n", strings.Join(argv, " "))
	return hostDocker.Exec(ctx, dockerx.ExecSpec{
		Container: cname,
		User:      agent.HostUser(),
		Argv:      argv,
	})
}
