package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestParseEmpty(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("gbx"),
		kong.Description("glovebox host CLI"),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	// With visible subcommands present, parsing an empty argv must error
	// asking for a command. This is the kong-default behavior.
	if _, err := parser.Parse([]string{}); err == nil {
		t.Fatalf("Parse([]): want error requesting a subcommand, got nil")
	}
}

func TestHelpMentionsName(t *testing.T) {
	var cli CLI
	var stdout bytes.Buffer
	parser, err := kong.New(&cli,
		kong.Name("gbx"),
		kong.Description("glovebox host CLI"),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Writers(&stdout, &stdout),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, _ = parser.Parse([]string{"--help"})
	if !strings.Contains(stdout.String(), "gbx") {
		t.Fatalf("help output missing binary name: %q", stdout.String())
	}
}
