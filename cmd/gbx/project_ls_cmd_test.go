package main

import (
	"testing"
	"time"

	"github.com/okulik/glovebox/internal/dockerx"
)

func TestShortContainerName(t *testing.T) {
	pid := "aaaa1111bbbb"
	cases := []struct{ in, want string }{
		{"glovebox-agent-" + pid, "agent"},
		{"glovebox-stack-" + pid + "-redis", "redis"},
		{"glovebox-stack-" + pid + "-neo4j", "neo4j"},
		{"glovebox-egress-proxy", "glovebox-egress-proxy"}, // non-matching: unchanged
	}
	for _, c := range cases {
		if got := shortContainerName(c.in, pid); got != c.want {
			t.Errorf("shortContainerName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatRelAge(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ts   time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-49 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := formatRelAge(c.ts, now); got != c.want {
			t.Errorf("formatRelAge(%v) = %q, want %q", c.ts, got, c.want)
		}
	}
}

func TestDeriveTag(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	built := now.Add(-3 * time.Hour).UTC().Format(dockerx.ImageCreatedLabelFormat)

	if got := deriveTag(map[string]string{"io.glovebox.test": "1"}, now); got != "test" {
		t.Errorf("test-only tag = %q, want %q", got, "test")
	}
	if got := deriveTag(map[string]string{"io.glovebox.image.created": built}, now); got != "built 3h ago" {
		t.Errorf("built tag = %q, want %q", got, "built 3h ago")
	}
	if got := deriveTag(map[string]string{"io.glovebox.test": "1", "io.glovebox.image.created": built}, now); got != "test, built 3h ago" {
		t.Errorf("combined tag = %q, want %q", got, "test, built 3h ago")
	}
	if got := deriveTag(map[string]string{"io.glovebox.image.created": "not-a-timestamp"}, now); got != "built" {
		t.Errorf("unparseable timestamp tag = %q, want %q", got, "built")
	}
	if got := deriveTag(map[string]string{"foo": "bar"}, now); got != "" {
		t.Errorf("no-relevant-labels tag = %q, want empty", got)
	}
}
