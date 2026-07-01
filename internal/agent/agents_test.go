package agent

import (
	"sort"
	"testing"
)

// installSpecs carries per-agent install/update commands keyed by name. Its key
// set must equal the canonical Names, or `gbx run/update <agent>` would accept
// or reject the wrong agents.
func TestInstallSpecsMatchNames(t *testing.T) {
	got := make([]string, 0, len(installSpecs))
	for k := range installSpecs {
		got = append(got, k)
	}
	assertSameSet(t, "installSpecs keys", got, Names)
}

func TestSupported(t *testing.T) {
	for _, n := range Names {
		if !Supported(n) {
			t.Errorf("Supported(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"", "Claude", "cursor", "codexx"} {
		if Supported(n) {
			t.Errorf("Supported(%q) = true, want false", n)
		}
	}
}

// assertSameSet fails if got and want don't contain the same elements.
func assertSameSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Fatalf("%s: got %v, want %v", label, g, w)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("%s: got %v, want %v", label, g, w)
		}
	}
}
