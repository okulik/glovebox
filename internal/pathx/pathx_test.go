package pathx

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUnderBaseAcceptsPathInside(t *testing.T) {
	base := t.TempDir()
	candidate := filepath.Join(base, "sub", "file.txt")
	got, err := UnderBase(base, candidate)
	if err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("returned path should start with base; got %q want prefix %q", got, base)
	}
}

func TestUnderBaseAcceptsPathEqualToBase(t *testing.T) {
	base := t.TempDir()
	if _, err := UnderBase(base, base); err != nil {
		t.Errorf("base == candidate should be ok: %v", err)
	}
}

func TestUnderBaseRejectsParentEscape(t *testing.T) {
	base := t.TempDir()
	// base/../sneaky resolves to a sibling of base - outside.
	candidate := filepath.Join(base, "..", "sneaky")
	if _, err := UnderBase(base, candidate); err == nil {
		t.Errorf("expected refusal for parent escape")
	}
}

func TestUnderBaseRejectsBoundaryPrefixMatch(t *testing.T) {
	// "/foo/bar" must NOT be considered inside "/foo/ba".
	if _, err := UnderBase("/foo/ba", "/foo/bar/file"); err == nil {
		t.Errorf("expected refusal: /foo/bar/file is not inside /foo/ba")
	}
}

func TestUnderBaseHandlesRelativeInputs(t *testing.T) {
	// Both inputs relative; both should resolve consistently against CWD.
	got, err := UnderBase(".", "./inside/x")
	if err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("result should be absolute, got %q", got)
	}
}

func TestUnderBaseTrailingSlashIsBoundaryAgnostic(t *testing.T) {
	base := t.TempDir()
	candidate := filepath.Join(base, "x")
	if _, err := UnderBase(base+string(filepath.Separator), candidate); err != nil {
		t.Errorf("trailing separator on base should be accepted: %v", err)
	}
}
