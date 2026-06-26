package stack

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

func newTestStack(t *testing.T) (*Stack, *dockerx.Fake, *dockerx.FakeHost) {
	t.Helper()
	host := dockerx.NewFakeHost()
	// Pretend both pulled images and the locally-built one already exist so
	// Up doesn't try to pull/build through the fake (FakeHost has no real
	// docker behind it). Tests that want to exercise the missing-image path
	// can clear these.
	host.Images = map[string]bool{
		config.ImageController:  true,
		config.ImageEgressProxy: true,
		config.ImageSocketProxy: true,
	}
	client := dockerx.NewFake()
	libexec := t.TempDir()
	cfgDir := t.TempDir()
	stateDir := t.TempDir()
	return &Stack{
		Host:      host,
		Client:    client,
		Libexec:   libexec,
		ConfigDir: cfgDir,
		StateDir:  stateDir,
	}, client, host
}

func TestUpCreatesNetworksAndContainers(t *testing.T) {
	s, client, _ := newTestStack(t)
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, n := range []string{config.NetworkInternal, config.NetworkControl, config.NetworkEgress} {
		if !client.Networks[n] {
			t.Errorf("network %s not created", n)
		}
	}
	for _, c := range []string{config.ContainerEgressProxy, config.ContainerSocketProxy, config.ContainerStackController} {
		fc, ok := client.Containers[c]
		if !ok {
			t.Errorf("container %s not created", c)
			continue
		}
		if fc.State != string(container.StateRunning) {
			t.Errorf("container %s state = %q, want running", c, fc.State)
		}
	}
}

func TestRebuildControllerAlwaysRebuildsAndRecreates(t *testing.T) {
	s, client, host := newTestStack(t)
	ctx := context.Background()
	// Stack already up; newTestStack marks all images present, so a plain Up
	// would NOT rebuild the controller.
	if err := s.Up(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if err := s.RebuildController(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("RebuildController: %v", err)
	}

	// It force-removed the controller container...
	if !slices.Contains(host.Calls, "rm -f "+config.ContainerStackController) {
		t.Errorf("expected force-remove of %s, host calls = %v", config.ContainerStackController, host.Calls)
	}
	// ...rebuilt the controller image unconditionally (the image already
	// existed, proving it doesn't take Up's ImageExists shortcut)...
	if host.LastBuild.Tag != config.ImageController {
		t.Errorf("expected controller image rebuild, last build tag = %q", host.LastBuild.Tag)
	}
	// ...and the controller ends up running.
	fc, ok := client.Containers[config.ContainerStackController]
	if !ok || fc.State != string(container.StateRunning) {
		t.Errorf("controller not running after rebuild: %+v", fc)
	}
}

func TestUpIsIdempotent(t *testing.T) {
	s, client, _ := newTestStack(t)
	ctx := context.Background()
	if err := s.Up(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Up #1: %v", err)
	}
	// Snapshot what was created. Second Up should leave everything as-is -
	// no extra network-connect entries, same container IDs.
	prevConnects := len(client.NetworkConnects)
	prevEgressID := client.Containers[config.ContainerEgressProxy].ID
	if err := s.Up(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Up #2: %v", err)
	}
	if got := len(client.NetworkConnects); got != prevConnects {
		t.Errorf("idempotent Up should not re-attach networks: prev=%d got=%d", prevConnects, got)
	}
	if client.Containers[config.ContainerEgressProxy].ID != prevEgressID {
		t.Errorf("idempotent Up should not recreate containers")
	}
}

func TestUpAttachesAdditionalNetworks(t *testing.T) {
	s, client, _ := newTestStack(t)
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// egress-proxy must end up on the egress network in addition to its
	// primary (internal). stack-controller needs control + egress on top of
	// its primary (internal). NetworkConnects only records the *additional*
	// attachments; primaries come from the EndpointsConfig passed at create.
	want := []struct {
		container, network string
	}{
		{config.ContainerEgressProxy, config.NetworkEgress},
		{config.ContainerStackController, config.NetworkControl},
		{config.ContainerStackController, config.NetworkEgress},
	}
	for _, w := range want {
		matched := slices.ContainsFunc(client.NetworkConnects, func(nc struct{ Container, Network string }) bool {
			return nc.Container == w.container && nc.Network == w.network
		})
		if !matched {
			t.Errorf("expected NetworkConnect(%s → %s); got %+v", w.container, w.network, client.NetworkConnects)
		}
	}
}

func TestIsRunningTrue(t *testing.T) {
	s, client, _ := newTestStack(t)
	client.Containers[config.ContainerEgressProxy] = dockerx.FakeContainer{ID: "x", State: string(container.StateRunning)}
	got, err := s.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !got {
		t.Error("want true when egress-proxy is running")
	}
}

func TestIsRunningFalseWhenAbsent(t *testing.T) {
	s, _, _ := newTestStack(t)
	got, err := s.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if got {
		t.Error("want false when egress-proxy is absent")
	}
}

func TestIsRunningFalseWhenExited(t *testing.T) {
	s, client, _ := newTestStack(t)
	client.Containers[config.ContainerEgressProxy] = dockerx.FakeContainer{ID: "x", State: string(container.StateExited)}
	got, err := s.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if got {
		t.Error("want false when egress-proxy is exited")
	}
}

func TestRestartProxyTargetsProxyContainer(t *testing.T) {
	s, _, host := newTestStack(t)
	if err := s.RestartProxy(context.Background()); err != nil {
		t.Fatalf("RestartProxy: %v", err)
	}
	want := "restart " + config.ContainerEgressProxy
	if !slices.Contains(host.Calls, want) {
		t.Errorf("expected call %q in %v", want, host.Calls)
	}
}

func TestUpBuildsControllerImageWhenMissing(t *testing.T) {
	s, _, host := newTestStack(t)
	delete(host.Images, config.ImageController) // force build path
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if host.LastBuild.Tag != config.ImageController {
		t.Errorf("expected BuildImage tag = %q, got %q", config.ImageController, host.LastBuild.Tag)
	}
}

func TestUpPullsMissingImages(t *testing.T) {
	s, client, host := newTestStack(t)
	delete(host.Images, config.ImageEgressProxy)
	delete(host.Images, config.ImageSocketProxy)
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, img := range []string{config.ImageEgressProxy, config.ImageSocketProxy} {
		if !slices.Contains(client.PulledImages, img) {
			t.Errorf("expected pull of %q, got %v", img, client.PulledImages)
		}
	}
}
