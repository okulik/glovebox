package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/okulik/glovebox/internal/agent"
)

// agentTable maps each known agent name to the path of its real binary inside
// the agent container. Five npm-installed agents go through ${NPM_PREFIX}/bin,
// two uv-installed agents go through ${UV_BIN_DIR}.
var agentTable = map[string]string{
	"claude":   agent.HomeNpm + "/bin/claude",
	"codex":    agent.HomeNpm + "/bin/codex",
	"gemini":   agent.HomeNpm + "/bin/gemini",
	"opencode": agent.HomeNpm + "/bin/opencode",
	"pi":       agent.HomeNpm + "/bin/pi",
	"aider":    agent.HomeLocalBin + "/aider",
	"hermes":   agent.HomeLocalBin + "/hermes",
}

// agentNames returns the wrapped agent names in sorted order, so help text
// and any other display stay in sync with agentTable.
func agentNames() []string {
	names := make([]string, 0, len(agentTable))
	for n := range agentTable {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// runner abstracts the side-effects of dispatch (filesystem stat, install
// subprocess, execve) so tests can verify the orchestration without actually
// performing them.
type runner interface {
	stat(path string) (os.FileInfo, error)
	install(name string) error
	exec(argv0 string, argv []string, env []string) error
}

// realRunner is the production implementation.
type realRunner struct{}

func (realRunner) stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (realRunner) install(name string) error {
	return agent.Install(context.Background(), agent.SystemExecutor{}, name)
}

func (realRunner) exec(argv0 string, argv []string, env []string) error {
	// argv0 comes from resolveAgent() which only returns paths from the
	// hardcoded agentTable allowlist; argv/env are the user's chosen flags
	// and environment for that agent, which is the whole point of the
	// wrapper. No injection vector that isn't already implicit in "the
	// operator chose to run this agent".
	//nolint:gosec // G702: argv0 is allowlist-restricted via resolveAgent.
	return syscall.Exec(argv0, argv, env)
}

// resolveAgent returns the real binary path for a known agent name, or an
// error if the name is not recognized.
func resolveAgent(name string) (string, error) {
	path, ok := agentTable[name]
	if !ok {
		return "", fmt.Errorf("unknown agent: %s", name)
	}
	return path, nil
}

// shouldChdir reports whether a wrapper should chdir to /workspace. Returns
// true unless pwd is already under /workspace.
func shouldChdir(pwd string) bool {
	if pwd == agent.WorkspaceDir {
		return false
	}
	if strings.HasPrefix(pwd, agent.WorkspaceDir+"/") {
		return false
	}
	return true
}

// dispatch is the entry point: read argv[0], look up the real binary, ensure
// it's installed, and exec it with the remaining args.
func dispatch() error {
	pwd, _ := os.Getwd()
	return dispatchWith(realRunner{}, os.Args[0], os.Args[1:], pwd)
}

// dispatchWith is the testable form: takes the invoking program name, the
// remaining args, the cwd, and a runner.
func dispatchWith(r runner, invoked string, args []string, pwd string) error {
	name := filepath.Base(invoked)
	realPath, err := resolveAgent(name)
	if err != nil {
		return err
	}

	if shouldChdir(pwd) {
		if err := os.Chdir(agent.WorkspaceDir); err != nil {
			return fmt.Errorf("chdir %s: %w", agent.WorkspaceDir, err)
		}
	}

	if _, err := r.stat(realPath); os.IsNotExist(err) {
		if instErr := r.install(name); instErr != nil {
			return fmt.Errorf("install %s: %w", name, instErr)
		}
	}

	argv := append([]string{name}, args...)
	return r.exec(realPath, argv, os.Environ())
}
