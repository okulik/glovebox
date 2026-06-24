package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var HermesTagResolver = defaultHermesTag

func defaultHermesTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/NousResearch/hermes-agent/releases/latest", nil)
	if err != nil {
		return "", fmt.Errorf("build hermes release request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch hermes latest release: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read hermes release body: %w", err)
	}
	m := regexp.MustCompile(`"tag_name"\s*:\s*"([^"]+)"`).FindSubmatch(body)
	if len(m) < 2 {
		return "", errors.New("could not extract tag_name from hermes release JSON")
	}
	return string(m[1]), nil
}

type installSpec struct {
	install func(ctx context.Context) ([]string, error)
	update  func(ctx context.Context) ([]string, error)
}

var installSpecs = map[string]installSpec{
	"claude": {
		install: staticCmd("npm", "install", "-g", "@anthropic-ai/claude-code"),
		update:  staticCmd("npm", "install", "-g", "@anthropic-ai/claude-code@latest"),
	},
	"codex": {
		install: staticCmd("npm", "install", "-g", "@openai/codex"),
		update:  staticCmd("npm", "install", "-g", "@openai/codex@latest"),
	},
	"opencode": {
		install: staticCmd("npm", "install", "-g", "opencode-ai"),
		update:  staticCmd("npm", "install", "-g", "opencode-ai@latest"),
	},
	"pi": {
		install: staticCmd("npm", "install", "-g", "@earendil-works/pi-coding-agent"),
		update:  staticCmd("npm", "install", "-g", "@earendil-works/pi-coding-agent@latest"),
	},
	"gemini": {
		install: staticCmd("npm", "install", "-g", "@google/gemini-cli"),
		update:  staticCmd("npm", "install", "-g", "@google/gemini-cli@latest"),
	},
	"aider": {
		install: staticCmd("uv", "tool", "install", "aider-chat"),
		update:  staticCmd("uv", "tool", "upgrade", "aider-chat"),
	},
	"hermes": {
		install: hermesCmd(),
		update:  hermesCmd("--reinstall"),
	},
}

func staticCmd(parts ...string) func(context.Context) ([]string, error) {
	cp := append([]string(nil), parts...)
	return func(context.Context) ([]string, error) { return cp, nil }
}

func hermesCmd(extra ...string) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		tag, err := HermesTagResolver(ctx)
		if err != nil {
			return nil, err
		}
		args := []string{"uv", "tool", "install"}
		args = append(args, extra...)
		args = append(args, "git+https://github.com/NousResearch/hermes-agent@"+tag)
		return args, nil
	}
}

// InstallArgv returns the install command for `name` as an argv slice, ready
// to feed into exec.Command or a docker exec spec. Returns an error if name
// is unknown or if a per-agent resolver (e.g. the hermes tag lookup) fails.
func InstallArgv(ctx context.Context, name string) ([]string, error) {
	return argvForSpec(ctx, name, func(s installSpec) func(context.Context) ([]string, error) { return s.install })
}

// UpdateArgv returns the upgrade command for `name` as an argv slice.
func UpdateArgv(ctx context.Context, name string) ([]string, error) {
	return argvForSpec(ctx, name, func(s installSpec) func(context.Context) ([]string, error) { return s.update })
}

func argvForSpec(ctx context.Context, name string, pick func(installSpec) func(context.Context) ([]string, error)) ([]string, error) {
	spec, ok := installSpecs[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", name)
	}
	return pick(spec)(ctx)
}

// Install runs the per-agent install command via exe. Used by gbxa (the
// in-container admin binary) to lazy-install an agent the first time the
// user invokes it inside the container. Host-driven install/update goes
// through InstallArgv/UpdateArgv + dockerx.Host.Exec instead.
func Install(ctx context.Context, exe Executor, name string) error {
	return runSpec(ctx, exe, name, InstallArgv)
}

// Update runs the per-agent upgrade command via exe. Same shape as Install -
// used by gbxa's `gbxa update <agent>` flow; host-side `gbx update <agent>`
// uses UpdateArgv + dockerx.Host.Exec.
func Update(ctx context.Context, exe Executor, name string) error {
	return runSpec(ctx, exe, name, UpdateArgv)
}

func runSpec(ctx context.Context, exe Executor, name string, argv func(context.Context, string) ([]string, error)) error {
	cmd, err := argv(ctx, name)
	if err != nil {
		return err
	}
	if exe == nil {
		exe = SystemExecutor{}
	}
	// When the executor supports streaming (production SystemExecutor does),
	// pipe stdout/stderr through so users see npm/uv progress live instead
	// of a long silent wait followed by a single dump.
	if s, ok := exe.(Streamer); ok {
		fmt.Fprintf(os.Stderr, "$ %s\n", strings.Join(cmd, " "))
		if rerr := s.RunStreaming(ctx, cmd[0], cmd[1:]...); rerr != nil {
			return fmt.Errorf("%s: %w", name, rerr)
		}
		return nil
	}
	out, rerr := exe.Run(ctx, cmd[0], cmd[1:]...)
	if rerr != nil {
		return fmt.Errorf("%s: %w (output: %s)", name, rerr, string(out))
	}
	return nil
}
