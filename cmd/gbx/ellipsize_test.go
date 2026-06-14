package main

import (
	"strings"
	"testing"
)

func TestEllipsizeLeft(t *testing.T) {
	// Exact-value cases for short / boundary inputs.
	exact := []struct {
		name string
		in   string
		want string
		n    int
	}{
		{"shorter than cap untouched", "/Users/orest/dev/projects/foo", "/Users/orest/dev/projects/foo", 65},
		{"equal to cap untouched", "abcdefghij", "abcdefghij", 10},
		{"n less than ellipsis: bare tail", "abcdefgh", "gh", 2},
		{"n equal to ellipsis: bare tail", "abcdefgh", "fgh", 3},
		{"unicode counted in runes not bytes", "αβγδεζηθικ", "...ικ", 5},
	}
	for _, c := range exact {
		t.Run(c.name, func(t *testing.T) {
			if got := ellipsizeLeft(c.in, c.n); got != c.want {
				t.Errorf("ellipsizeLeft(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
			}
		})
	}

	// Property checks for the typical long-path case.
	t.Run("long path truncated to n runes with leading ellipsis and basename intact", func(t *testing.T) {
		in := "/Users/orest/dev/projects/very-long-name-that-exceeds-the-column-width"
		got := ellipsizeLeft(in, 40)
		if l := len([]rune(got)); l != 40 {
			t.Errorf("rune length = %d, want 40", l)
		}
		if !strings.HasPrefix(got, "...") {
			t.Errorf("missing leading ellipsis: %q", got)
		}
		if !strings.HasSuffix(got, "column-width") {
			t.Errorf("basename suffix lost: %q", got)
		}
	})
}
