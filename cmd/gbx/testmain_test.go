package main

import (
	"bytes"
	"testing"

	"github.com/alecthomas/kong"
)

// runCLI parses argv against CLI and returns (stdoutText, stderrText, exitCode).
// kong.Exit is captured; on parser error we synthesize exit 1.
func runCLI(t *testing.T, argv ...string) (string, string, int) {
	t.Helper()
	var cli CLI
	var stdout, stderr bytes.Buffer
	exit := -1
	parser, err := kong.New(&cli,
		kong.Name("gbx"),
		kong.Description("glovebox host CLI"),
		kong.Vars{"version": "0.0.0-dev"},
		kong.Writers(&stdout, &stderr),
		kong.Exit(func(code int) { exit = code }),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	ctx, err := parser.Parse(argv)
	if err != nil {
		stderr.WriteString(err.Error() + "\n")
		return stdout.String(), stderr.String(), 1
	}
	if err := ctx.Run(); err != nil {
		stderr.WriteString(err.Error() + "\n")
		return stdout.String(), stderr.String(), 1
	}
	if exit == -1 {
		exit = 0
	}
	return stdout.String(), stderr.String(), exit
}
