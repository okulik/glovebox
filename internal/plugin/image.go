package plugin

import "context"

// ImageProber is the subset of dockerx.HostClient that SelectImage needs.
type ImageProber interface {
	ImageExists(ctx context.Context, image string) bool
}

// SelectImage returns the image a project's agent container should run from.
func SelectImage(ctx context.Context, prober ImageProber, base, pid string) string {
	if prober == nil {
		return base
	}
	derived := DerivedImageTag(base, pid)
	if prober.ImageExists(ctx, derived) {
		return derived
	}
	return base
}
