package stack

import (
	"bytes"
	"context"
	"slices"
	"testing"

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
		ImageController:  true,
		ImageEgressProxy: true,
		ImageSocketProxy: true,
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
	for _, n := range []string{NetworkInternal, NetworkControl, NetworkEgress} {
		if !client.Networks[n] {
			t.Errorf("network %s not created", n)
		}
	}
	for _, c := range []string{ContainerEgressProxy, ContainerSocketProxy, ContainerController} {
		fc, ok := client.Containers[c]
		if !ok {
			t.Errorf("container %s not created", c)
			continue
		}
		if fc.State != "running" {
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
	if !slices.Contains(host.Calls, "rm -f "+ContainerController) {
		t.Errorf("expected force-remove of %s, host calls = %v", ContainerController, host.Calls)
	}
	// ...rebuilt the controller image unconditionally (the image already
	// existed, proving it doesn't take Up's ImageExists shortcut)...
	if host.LastBuild.Tag != ImageController {
		t.Errorf("expected controller image rebuild, last build tag = %q", host.LastBuild.Tag)
	}
	// ...and the controller ends up running.
	fc, ok := client.Containers[ContainerController]
	if !ok || fc.State != "running" {
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
	prevEgressID := client.Containers[ContainerEgressProxy].ID
	if err := s.Up(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("Up #2: %v", err)
	}
	if got := len(client.NetworkConnects); got != prevConnects {
		t.Errorf("idempotent Up should not re-attach networks: prev=%d got=%d", prevConnects, got)
	}
	if client.Containers[ContainerEgressProxy].ID != prevEgressID {
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
		{ContainerEgressProxy, NetworkEgress},
		{ContainerController, NetworkControl},
		{ContainerController, NetworkEgress},
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
	client.Containers[ContainerEgressProxy] = dockerx.FakeContainer{ID: "x", State: "running"}
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
	client.Containers[ContainerEgressProxy] = dockerx.FakeContainer{ID: "x", State: "exited"}
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
	want := "restart " + ContainerEgressProxy
	if !slices.Contains(host.Calls, want) {
		t.Errorf("expected call %q in %v", want, host.Calls)
	}
}

func TestUpBuildsControllerImageWhenMissing(t *testing.T) {
	s, _, host := newTestStack(t)
	delete(host.Images, ImageController) // force build path
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if host.LastBuild.Tag != ImageController {
		t.Errorf("expected BuildImage tag = %q, got %q", ImageController, host.LastBuild.Tag)
	}
}

func TestUpPullsMissingImages(t *testing.T) {
	s, client, host := newTestStack(t)
	delete(host.Images, ImageEgressProxy)
	delete(host.Images, ImageSocketProxy)
	if err := s.Up(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, img := range []string{ImageEgressProxy, ImageSocketProxy} {
		if !slices.Contains(client.PulledImages, img) {
			t.Errorf("expected pull of %q, got %v", img, client.PulledImages)
		}
	}
}
