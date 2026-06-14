package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestParseVersionFlag(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("gbxa"),
		kong.Description(appDescription()),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	// gbxa has no subcommands today (the former `update` moved to `gbx
	// update`); the only flag is --version. Parsing it should succeed.
	if _, err := parser.Parse([]string{"--version"}); err != nil {
		t.Fatalf("Parse([--version]): %v", err)
	}
}

func TestBareInvocationPrintsHelp(t *testing.T) {
	var cli CLI
	var stdout bytes.Buffer
	parser, err := kong.New(&cli,
		kong.Name("gbxa"),
		kong.Description(appDescription()),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Writers(&stdout, &stdout),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	// Bare `gbxa` (no args) selects no command; main() prints usage and exits
	// 0 instead of erroring with "no command selected".
	ctx, err := parser.Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if ctx.Command() != "" {
		t.Fatalf("expected no command selected, got %q", ctx.Command())
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatalf("PrintUsage: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Usage: gbxa") {
		t.Fatalf("bare invocation should print usage: %q", out)
	}
	// The help should list every wrapped agent name so a bare `gbxa` explains
	// what the multi-call binary is for.
	for _, name := range agentNames() {
		if !strings.Contains(out, name) {
			t.Errorf("help missing wrapped agent %q: %q", name, out)
		}
	}
}

func TestHelpMentionsName(t *testing.T) {
	var cli CLI
	var stdout bytes.Buffer
	parser, err := kong.New(&cli,
		kong.Name("gbxa"),
		kong.Description(appDescription()),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Writers(&stdout, &stdout),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, _ = parser.Parse([]string{"--help"})
	if !strings.Contains(stdout.String(), "gbxa") {
		t.Fatalf("help output missing binary name: %q", stdout.String())
	}
}
