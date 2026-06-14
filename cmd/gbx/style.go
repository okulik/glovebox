package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// styler emits ANSI color escapes when the destination writer is a TTY,
// and is a no-op otherwise (so `gbx ls > file` or `gbx ls | grep` keep
// clean plain text).
type styler struct {
	enabled bool
}

// newStyler returns a styler whose decorations fire only when w is a real
// terminal file descriptor. Bytes.Buffer (used in tests) and pipes both
// resolve to enabled=false.
func newStyler(w io.Writer) styler {
	f, ok := w.(*os.File)
	if !ok {
		return styler{}
	}
	return styler{enabled: term.IsTerminal(int(f.Fd()))}
}

const (
	csiReset  = "\x1b[0m"
	csiBold   = "\x1b[1m"
	csiDim    = "\x1b[2m"
	csiRed    = "\x1b[31m"
	csiGreen  = "\x1b[32m"
	csiYellow = "\x1b[33m"
)

func (s styler) wrap(code, text string) string {
	if !s.enabled {
		return text
	}
	return code + text + csiReset
}

func (s styler) bold(text string) string   { return s.wrap(csiBold, text) }
func (s styler) dim(text string) string    { return s.wrap(csiDim, text) }
func (s styler) red(text string) string    { return s.wrap(csiRed, text) }
func (s styler) green(text string) string  { return s.wrap(csiGreen, text) }
func (s styler) yellow(text string) string { return s.wrap(csiYellow, text) }

// status pads text to width and colors it by meaning. Handles the strings
// we surface in `gbx ls`: AgentStatus ("running"/"exited"/"absent"), the
// StackStatus phrases ("no stack" / "N containers"), and Docker's free-form
// container Status ("Up 5 minutes (healthy)" / "Exited (1) ago"). Pads
// first so ANSI bytes don't corrupt column alignment.
func (s styler) status(text string, width int) string {
	padded := fmt.Sprintf("%-*s", width, text)
	switch {
	case strings.Contains(text, "unhealthy"):
		return s.red(padded)
	case strings.Contains(text, "(healthy)"),
		text == "running",
		strings.HasPrefix(text, "Up "):
		return s.green(padded)
	case text == "exited", strings.HasPrefix(text, "Exited"):
		return s.yellow(padded)
	case text == "", text == "absent", text == "no stack":
		return s.dim(padded)
	}
	return padded
}
