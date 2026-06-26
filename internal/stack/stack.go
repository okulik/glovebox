// Package stack owns the singleton egress-proxy + socket-proxy + stack-controller
// trio that every project shares. It is the typed Go successor to
// docker/compose.yml - same three services, same three networks, same
// healthchecks, but driven by the moby SDK directly so there's no need for
// the docker-compose CLI on the host.
package stack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

const (
	// defaultHealthTimeout bounds how long Up waits for each healthchecked
	// container to reach "healthy".
	defaultHealthTimeout = 60 * time.Second
)

// Stack drives the singleton control-plane stack via the Docker Engine API.
type Stack struct {
	Host      dockerx.HostClient
	Client    dockerx.ControllerClient
	Libexec   string
	ConfigDir string
	StateDir  string
}

// FromEnv builds a Stack from the GBX_* env contract resolved by gbxconfig.
func FromEnv(host dockerx.HostClient, client dockerx.ControllerClient) (*Stack, error) {
	gcfg := config.GbxFromEnv()
	if gcfg.Libexec == "" {
		return nil, errors.New("GBX_LIBEXEC not set")
	}
	return &Stack{
		Host:      host,
		Client:    client,
		Libexec:   gcfg.Libexec,
		ConfigDir: gcfg.ConfigDir,
		StateDir:  gcfg.StateDir,
	}, nil
}

// IsRunning reports whether the egress-proxy container is in the string(container.StateRunning)
// state - used as a cheap probe to decide whether Up needs to do work.
func (s *Stack) IsRunning(ctx context.Context) (bool, error) {
	_, state, err := s.Client.ContainerByName(ctx, config.ContainerEgressProxy)
	if err != nil {
		return false, err
	}
	return state == string(container.StateRunning), nil
}

// RestartProxy restarts the egress-proxy. This is what `gbx allow` runs
// after appending a new domain to the allowlist so squid re-reads the file.
func (s *Stack) RestartProxy(ctx context.Context) error {
	return s.Host.RestartContainer(ctx, config.ContainerEgressProxy)
}

// ProxyLogs streams the squid access log to the caller's terminal. Mirrors
// the previous `compose exec egress-proxy tail -F ...` UX.
func (s *Stack) ProxyLogs(ctx context.Context) error {
	return s.Host.Exec(ctx, dockerx.ExecSpec{
		Container: config.ContainerEgressProxy,
		Argv:      []string{"tail", "-F", "/var/log/squid/access.log"},
	})
}

// controllerLogTail caps the backlog shown before following, mirroring the
// last-N-lines feel of `tail -F` rather than dumping the whole history.
const controllerLogTail = 200

// ControllerLogs streams the stack-controller's HTTP-server stdout/stderr to
// w, following live. Unlike ProxyLogs it can't exec `tail`: the controller's
// image is distroless (no shell, no tail), so logs come through the Engine
// API instead.
func (s *Stack) ControllerLogs(ctx context.Context, w io.Writer) error {
	return s.Host.ContainerLogs(ctx, config.ContainerStackController, controllerLogTail, true, w, w)
}

// Up brings the singleton stack to a healthy steady state: networks exist,
// images are present (built/pulled), containers are running, healthchecks
// are passing. Progress and pull/build output stream to w; pass os.Stderr
// to match the old `compose up -d --wait` UX.
func (s *Stack) Up(ctx context.Context, w io.Writer) error {
	if w == nil {
		w = os.Stderr
	}

	for _, n := range []struct {
		name     string
		internal bool
	}{
		{config.NetworkInternal, true},
		{config.NetworkControl, true},
		{config.NetworkEgress, false},
	} {
		if err := s.Client.EnsureNetwork(ctx, n.name, n.internal); err != nil {
			return fmt.Errorf("ensure network %s: %w", n.name, err)
		}
	}

	if !s.Host.ImageExists(ctx, config.ImageController) {
		fmt.Fprintln(w, "Building glovebox-stack-controller:local (one-time, ~30s)...")
		if err := s.buildController(ctx, w); err != nil {
			return fmt.Errorf("build stack-controller image: %w", err)
		}
	}
	for _, img := range []string{config.ImageSocketProxy, config.ImageEgressProxy} {
		if s.Host.ImageExists(ctx, img) {
			continue
		}
		fmt.Fprintf(w, "Pulling %s...\n", img)
		if err := s.Client.PullImageStream(ctx, img, w); err != nil {
			return fmt.Errorf("pull %s: %w", img, err)
		}
	}

	// Container creation order matches compose's depends_on graph:
	// socket-proxy is required for stack-controller to talk to docker, so it
	// must be healthy first.
	if err := s.ensureSocketProxy(ctx); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx, config.ContainerSocketProxy, defaultHealthTimeout); err != nil {
		return err
	}
	if err := s.ensureEgressProxy(ctx); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx, config.ContainerEgressProxy, defaultHealthTimeout); err != nil {
		return err
	}
	// stack-controller has no healthcheck; just create+start.
	return s.ensureController(ctx)
}

func (s *Stack) buildController(ctx context.Context, w io.Writer) error {
	return s.Host.BuildImage(ctx, dockerx.BuildSpec{
		Tag:        config.ImageController,
		Dockerfile: filepath.Join(s.Libexec, "docker", "controller.Dockerfile"),
		Context:    s.Libexec,
		Out:        w,
		Err:        w,
	})
}

// RebuildController force-rebuilds the stack-controller image from current
// source and recreates its container.
func (s *Stack) RebuildController(ctx context.Context, w io.Writer) error {
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintln(w, "Removing the stack-controller container...")
	if err := s.Host.ForceRemoveContainer(ctx, config.ContainerStackController); err != nil {
		return fmt.Errorf("remove %s: %w", config.ContainerStackController, err)
	}
	fmt.Fprintf(w, "Rebuilding %s from source...\n", config.ImageController)
	if err := s.buildController(ctx, w); err != nil {
		return fmt.Errorf("build stack-controller image: %w", err)
	}
	return s.Up(ctx, w)
}

// waitHealthy polls a container's HealthState until "healthy" or timeout.
func (s *Stack) waitHealthy(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		id, _, err := s.Client.ContainerByName(ctx, name)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", name, err)
		}
		if id != "" {
			h, err := s.Client.HealthState(ctx, id)
			if err != nil {
				return fmt.Errorf("health %s: %w", name, err)
			}
			if h == "" || h == "healthy" {
				return nil
			}
			if h == "unhealthy" {
				return fmt.Errorf("%s is unhealthy", name)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become healthy in %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Stack) ensureSocketProxy(ctx context.Context) error {
	id, state, err := s.Client.ContainerByName(ctx, config.ContainerSocketProxy)
	if err != nil {
		return err
	}
	if id == "" {
		cfg, hostCfg, netCfg := socketProxyConfig()
		if _, err := s.Client.CreateContainerRaw(ctx, config.ContainerSocketProxy, cfg, hostCfg, netCfg); err != nil {
			return fmt.Errorf("create %s: %w", config.ContainerSocketProxy, err)
		}
	}
	if state == string(container.StateRunning) {
		return nil
	}
	return s.Client.StartContainer(ctx, config.ContainerSocketProxy)
}

func (s *Stack) ensureEgressProxy(ctx context.Context) error {
	id, state, err := s.Client.ContainerByName(ctx, config.ContainerEgressProxy)
	if err != nil {
		return err
	}
	if id == "" {
		cfg, hostCfg, netCfg := s.egressProxyConfig()
		if _, err := s.Client.CreateContainerRaw(ctx, config.ContainerEgressProxy, cfg, hostCfg, netCfg); err != nil {
			return fmt.Errorf("create %s: %w", config.ContainerEgressProxy, err)
		}
		// Egress-proxy needs a second network for outbound traffic; the
		// primary (internal) is already wired by NetworkingConfig at create.
		if err := s.Client.ConnectNetwork(ctx, config.ContainerEgressProxy, config.NetworkEgress); err != nil {
			return fmt.Errorf("attach %s to %s: %w", config.ContainerEgressProxy, config.NetworkEgress, err)
		}
	}
	if state == string(container.StateRunning) {
		return nil
	}
	return s.Client.StartContainer(ctx, config.ContainerEgressProxy)
}

func (s *Stack) ensureController(ctx context.Context) error {
	// Bind-target dir for /state must exist on the host or the bind mount
	// fails at create.
	if err := os.MkdirAll(filepath.Join(s.StateDir, "controller"), 0o755); err != nil {
		return fmt.Errorf("create state/controller: %w", err)
	}
	id, state, err := s.Client.ContainerByName(ctx, config.ContainerStackController)
	if err != nil {
		return err
	}
	if id == "" {
		cfg, hostCfg, netCfg, err := s.controllerConfig()
		if err != nil {
			return err
		}
		if _, err := s.Client.CreateContainerRaw(ctx, config.ContainerStackController, cfg, hostCfg, netCfg); err != nil {
			return fmt.Errorf("create %s: %w", config.ContainerStackController, err)
		}
		// Controller needs both glovebox-control (to reach socket-proxy)
		// and glovebox-egress (to pull project images from public registries).
		for _, n := range []string{config.NetworkControl, config.NetworkEgress} {
			if err := s.Client.ConnectNetwork(ctx, config.ContainerStackController, n); err != nil {
				return fmt.Errorf("attach %s to %s: %w", config.ContainerStackController, n, err)
			}
		}
	}
	if state == string(container.StateRunning) {
		return nil
	}
	return s.Client.StartContainer(ctx, config.ContainerStackController)
}

func socketProxyConfig() (*container.Config, *container.HostConfig, *network.NetworkingConfig) {
	cfg := &container.Config{
		Image:    config.ImageSocketProxy,
		Hostname: config.HostnameSocketProxy,
		Env: []string{
			"CONTAINERS=1", "NETWORKS=1", "VOLUMES=1", "IMAGES=1", "VERSION=1",
			"INFO=0", "EXEC=0", "AUTH=0", "SECRETS=0", "SERVICES=0", "SWARM=0", "SYSTEM=0",
			"POST=1",
		},
		Healthcheck: &container.HealthConfig{
			Test:        []string{"CMD-SHELL", "wget -qO- http://127.0.0.1:2375/version || exit 1"},
			Interval:    3 * time.Second,
			Timeout:     2 * time.Second,
			Retries:     10,
			StartPeriod: 5 * time.Second,
		},
	}
	hostCfg := &container.HostConfig{
		Binds:         []string{"/var/run/docker.sock:/var/run/docker.sock:ro"},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			config.NetworkControl: {Aliases: []string{config.HostnameSocketProxy}},
		},
	}
	return cfg, hostCfg, netCfg
}

func (s *Stack) egressProxyConfig() (*container.Config, *container.HostConfig, *network.NetworkingConfig) {
	cfg := &container.Config{
		Image:    config.ImageEgressProxy,
		Hostname: config.HostnameProxy,
		// `squid -k check` confirms the running squid process via its PID file
		// without opening a connection to the proxy port. The previous
		// `echo >/dev/tcp/127.0.0.1/3128` probe opened and closed a TCP
		// connection without sending a request, which squid logged as a noisy
		// "NONE_NONE/000 ... error:transaction-end-before-headers" line on
		// every interval (and access_log ACLs can't suppress pre-header
		// aborts). This check is silent.
		Healthcheck: &container.HealthConfig{
			Test:        []string{"CMD", "squid", "-k", "check"},
			Interval:    3 * time.Second,
			Timeout:     2 * time.Second,
			Retries:     10,
			StartPeriod: 5 * time.Second,
		},
	}
	hostCfg := &container.HostConfig{
		Binds: []string{
			filepath.Join(s.Libexec, config.DockerDirName, "proxy", "squid.conf") + ":/etc/squid/squid.conf:ro",
			filepath.Join(s.ConfigDir, "allowlist.txt") + ":/etc/squid/allowlist.txt:ro",
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			config.NetworkInternal: {Aliases: []string{config.HostnameProxy}},
		},
	}
	return cfg, hostCfg, netCfg
}

func (s *Stack) controllerConfig() (*container.Config, *container.HostConfig, *network.NetworkingConfig, error) {
	// Single source of truth for the CONTROLLER_* defaults: ControllerFromEnv
	// runs on the host here, picks up any operator override of the same env
	// vars, and hands the resolved values to the container we're about to
	// create. The controller process re-reads the same env on its side.
	ccfg := config.ControllerFromEnv()
	gcfg := config.GbxFromEnv()

	// The container port we publish is whatever the controller said it
	// would listen on via HostAddr (default ":7001"). The host port is the
	// loopback port gbx exposes externally (default "17001").
	containerPort, err := portFromAddr(ccfg.HostAddr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse CONTROLLER_HOST_ADDR: %w", err)
	}
	loopback, err := netip.ParseAddr("127.0.0.1")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse loopback: %w", err)
	}

	cfg := &container.Config{
		Image:    config.ImageController,
		Hostname: config.HostnameStackController,
		Env: []string{
			"CONTROLLER_DOCKER_HOST=" + ccfg.DockerHost,
			"CONTROLLER_STATE_DIR=" + ccfg.StateDir,
			"CONTROLLER_IMAGE_ALLOWLIST=" + ccfg.ImageAllowlistPath,
			"CONTROLLER_INTERNAL_ADDR=" + ccfg.InternalAddr,
			"CONTROLLER_HOST_ADDR=" + ccfg.HostAddr,
		},
		ExposedPorts: network.PortSet{containerPort: struct{}{}},
		// The controller's image is gcr.io/distroless/static-debian12:nonroot
		// (no shell, no wget). We can't use CMD-SHELL here, but the image
		// does have /controller itself, so we shell-out to the binary's
		// own --healthcheck subcommand, which probes /health on the
		// internal listener and exits 0/1.
		Healthcheck: &container.HealthConfig{
			Test:        []string{"CMD", "/controller", "--healthcheck"},
			Interval:    3 * time.Second,
			Timeout:     2 * time.Second,
			Retries:     10,
			StartPeriod: 5 * time.Second,
		},
	}
	hostCfg := &container.HostConfig{
		Binds: []string{
			filepath.Join(s.StateDir, "controller") + ":/state",
			filepath.Join(s.Libexec, config.DockerDirName, "image-allowlist.txt") + ":/config/image-allowlist.txt:ro",
		},
		PortBindings: network.PortMap{
			containerPort: []network.PortBinding{{HostIP: loopback, HostPort: gcfg.ControllerHostPort}},
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			config.NetworkInternal: {Aliases: []string{config.HostnameStackController}},
		},
	}
	return cfg, hostCfg, netCfg, nil
}

// portFromAddr extracts the port out of a Go-style listen address such as
// ":7001", "0.0.0.0:7001", or "127.0.0.1:7001". Returns it as a typed
// network.Port (always TCP) suitable for ExposedPorts / PortBindings.
func portFromAddr(addr string) (network.Port, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return network.Port{}, err
	}
	return network.ParsePort(port + "/tcp")
}
