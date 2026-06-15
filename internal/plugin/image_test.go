package plugin_test

import (
	"context"
	"testing"

	"github.com/okulik/glovebox/internal/plugin"
)

type fakeProber struct{ have map[string]bool }

func (f fakeProber) ImageExists(_ context.Context, image string) bool { return f.have[image] }

func TestSelectImage(t *testing.T) {
	ctx := context.Background()
	base := "glovebox-agent:local"
	pid := "abc123"
	derived := plugin.DerivedImageTag(base, pid)

	// Derived image present -> use it.
	got := plugin.SelectImage(ctx, fakeProber{have: map[string]bool{derived: true}}, base, pid)
	if got != derived {
		t.Errorf("got %q, want derived %q", got, derived)
	}

	// Derived absent -> fall back to base.
	got = plugin.SelectImage(ctx, fakeProber{have: map[string]bool{}}, base, pid)
	if got != base {
		t.Errorf("got %q, want base %q", got, base)
	}

	// Nil prober -> base (defensive).
	if got := plugin.SelectImage(ctx, nil, base, pid); got != base {
		t.Errorf("nil prober: got %q, want base %q", got, base)
	}
}
