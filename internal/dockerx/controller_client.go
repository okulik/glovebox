package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
)

// isNotFound matches the canonical moby SDK convention: any error type that
// implements `NotFound()` is treated as a 404 / "no such object". Using the
// behavioral interface keeps us decoupled from the unexported objectNotFoundError type.
func isNotFound(err error) bool {
	var nf interface{ NotFound() }
	return errors.As(err, &nf)
}

// ContainerSummary is the slice of container info we expose to callers.
//
//nolint:govet // fieldalignment: 8-byte savings irrelevant for a list-row DTO.
type ContainerSummary struct {
	ID string
	// Image is the image reference the container was created from, as the
	// docker CLI shows it in the IMAGE column (e.g. "glovebox-agent:local").
	Image string
	Name  string
	State string // "running", "exited", "created", etc.
	// Status is the free-form human-readable status the docker CLI shows
	// in the STATUS column, e.g. "Up 5 minutes" or "Up 5 minutes (healthy)".
	Status string
	// Labels mirrors the container's docker labels. May be nil.
	Labels map[string]string
}

// ControllerClient is the slice of Docker operations the controller uses.
// Defined as an interface so tests can mock it.
type ControllerClient interface {
	// EnsureNetwork creates a named network if missing. internal=true means
	// containers on the network have no internet route.
	EnsureNetwork(ctx context.Context, name string, internal bool) error
	RemoveNetwork(ctx context.Context, name string) error

	// PullImage pulls if missing. No-op if already present locally.
	PullImage(ctx context.Context, image string) error

	// PullImageStream pulls image and streams human-readable progress (one
	// line per JSON frame's `status` field, optionally prefixed with the
	// layer id) to w. Use this on first-run paths where a silent pull is
	// indistinguishable from a hang.
	PullImageStream(ctx context.Context, image string, w io.Writer) error

	// EnsureVolume creates a named volume if missing.
	EnsureVolume(ctx context.Context, name string) error
	RemoveVolume(ctx context.Context, name string) error

	// ListVolumesByPrefix returns all volume names whose name starts with prefix.
	ListVolumesByPrefix(ctx context.Context, prefix string) ([]string, error)

	// CreateContainer creates a container per spec, attached to networkName
	// with the spec's DNS aliases. Returns the container ID.
	CreateContainer(ctx context.Context, spec ContainerSpec, networkName string) (string, error)

	// CreateContainerRaw creates a container from already-typed Docker API
	// configs and returns the container ID. Use it when the caller (the
	// per-project agent today, image-restore tooling later) needs full
	// control of the container spec that the higher-level CreateContainer's
	// ContainerSpec doesn't cover.
	CreateContainerRaw(ctx context.Context, name string, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig) (string, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
	RemoveContainer(ctx context.Context, id string, force bool) error

	// ContainerByName returns the container ID and current state, or ("", "", nil) if missing.
	ContainerByName(ctx context.Context, name string) (id, state string, err error)

	// ListContainersByPrefix returns all containers (running and stopped) whose
	// name starts with the given prefix.
	ListContainersByPrefix(ctx context.Context, prefix string) ([]ContainerSummary, error)

	// HealthState returns the health status ("healthy", "unhealthy", "starting",
	// or "" if no healthcheck is configured).
	HealthState(ctx context.Context, id string) (string, error)

	// Logs streams a service's logs into w.
	Logs(ctx context.Context, id string, tailLines int, follow bool, w io.Writer) error

	// ConnectNetwork attaches an existing container to a network.
	ConnectNetwork(ctx context.Context, containerName, networkName string) error

	// NetworkContainerCount returns the number of containers attached to
	// the named network. exists=false (with err=nil) means the network is
	// absent; err is non-nil only on transport or API failures.
	NetworkContainerCount(ctx context.Context, name string) (count int, exists bool, err error)
}

// NewControllerClient returns a Client backed by the official Docker SDK, talking
// to dockerHost (e.g. "tcp://socket-proxy:2375").
func NewControllerClient(dockerHost string) (ControllerClient, error) {
	c, err := dockerclient.New(
		dockerclient.WithHost(dockerHost),
	)
	if err != nil {
		return nil, err
	}
	return &controllerClient{c: c}, nil
}

// NewControllerClientFromEnv returns a Client whose connection parameters come from
// DOCKER_HOST / DOCKER_API_VERSION / DOCKER_TLS_VERIFY in the environment.
// Use this on the host side, where the user's container runtime
// (OrbStack / Docker Desktop / Colima) configures things via env vars.
func NewControllerClientFromEnv() (ControllerClient, error) {
	c, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return nil, err
	}
	return &controllerClient{c: c}, nil
}

type controllerClient struct {
	c *dockerclient.Client
}

func (r *controllerClient) EnsureNetwork(ctx context.Context, name string, internal bool) error {
	res, err := r.c.NetworkList(ctx, dockerclient.NetworkListOptions{})
	if err != nil {
		return err
	}
	for _, n := range res.Items {
		if n.Name == name {
			return nil
		}
	}
	_, err = r.c.NetworkCreate(ctx, name, dockerclient.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: internal,
	})
	return err
}

func (r *controllerClient) RemoveNetwork(ctx context.Context, name string) error {
	_, err := r.c.NetworkRemove(ctx, name, dockerclient.NetworkRemoveOptions{})
	return err
}

func (r *controllerClient) PullImage(ctx context.Context, img string) error {
	// Skip if present.
	res, err := r.c.ImageList(ctx, dockerclient.ImageListOptions{})
	if err != nil {
		return err
	}
	for _, i := range res.Items {
		if slices.Contains(i.RepoTags, img) {
			return nil
		}
	}
	pull, err := r.c.ImagePull(ctx, img, dockerclient.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	// ImagePull returns nil even when the pull fails partway - the real error
	// arrives as a JSON frame in the progress stream. Decode each frame and
	// surface the first errorDetail/error.
	dec := json.NewDecoder(pull)
	for {
		var msg struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if derr := dec.Decode(&msg); derr != nil {
			if errors.Is(derr, io.EOF) {
				return nil
			}
			return derr
		}
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
		if msg.ErrorDetail.Message != "" {
			return errors.New(msg.ErrorDetail.Message)
		}
	}
}

func (r *controllerClient) PullImageStream(ctx context.Context, img string, w io.Writer) error {
	// Same skip-if-present check as PullImage so the streamed variant is
	// also free when the image is already there.
	res, err := r.c.ImageList(ctx, dockerclient.ImageListOptions{})
	if err != nil {
		return err
	}
	for _, i := range res.Items {
		if slices.Contains(i.RepoTags, img) {
			return nil
		}
	}
	pull, err := r.c.ImagePull(ctx, img, dockerclient.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	// Decode the JSON progress stream. Each frame has either:
	//   {"status":"Pulling fs layer", "id":"abc123"}
	//   {"progress":"[==>] 5MB/12MB", "id":"abc123"}
	//   {"error":"...", "errorDetail":{...}}
	// We surface status lines (optionally prefixed with id) and treat any
	// error frame as a terminal failure.
	dec := json.NewDecoder(pull)
	for {
		var msg struct {
			Status      string `json:"status"`
			ID          string `json:"id"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if derr := dec.Decode(&msg); derr != nil {
			if errors.Is(derr, io.EOF) {
				return nil
			}
			return derr
		}
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
		if msg.ErrorDetail.Message != "" {
			return errors.New(msg.ErrorDetail.Message)
		}
		if msg.Status == "" {
			continue
		}
		if msg.ID != "" {
			fmt.Fprintf(w, "%s: %s\n", msg.ID, msg.Status)
		} else {
			fmt.Fprintf(w, "%s\n", msg.Status)
		}
	}
}

func (r *controllerClient) EnsureVolume(ctx context.Context, name string) error {
	_, err := r.c.VolumeInspect(ctx, name, dockerclient.VolumeInspectOptions{})
	if err == nil {
		return nil
	}
	_, err = r.c.VolumeCreate(ctx, dockerclient.VolumeCreateOptions{Name: name})
	return err
}

func (r *controllerClient) RemoveVolume(ctx context.Context, name string) error {
	_, err := r.c.VolumeRemove(ctx, name, dockerclient.VolumeRemoveOptions{Force: false})
	return err
}

func (r *controllerClient) ListVolumesByPrefix(ctx context.Context, prefix string) ([]string, error) {
	res, err := r.c.VolumeList(ctx, dockerclient.VolumeListOptions{})
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, v := range res.Items {
		if strings.HasPrefix(v.Name, prefix) {
			out = append(out, v.Name)
		}
	}
	return out, nil
}

func (r *controllerClient) CreateContainer(ctx context.Context, spec ContainerSpec, networkName string) (string, error) {
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	mounts := make([]mount.Mount, 0, len(spec.Mounts))
	for _, m := range spec.Mounts {
		mounts = append(mounts, mount.Mount{Type: mount.TypeVolume, Source: m.VolumeName, Target: m.Target})
	}

	cc := &container.Config{Image: spec.Image, Env: env}
	if spec.Healthcheck != nil {
		cc.Healthcheck = &container.HealthConfig{
			Test:     spec.Healthcheck.Test,
			Interval: spec.Healthcheck.Interval,
			Timeout:  spec.Healthcheck.Timeout,
			Retries:  spec.Healthcheck.Retries,
		}
	}
	// Minimal capability set sufficient for typical service images whose
	// entrypoints start as root and drop to a service user (redis, postgres,
	// neo4j, etc.). SETUID/SETGID/SETPCAP are required by `setpriv`/`gosu`;
	// DAC_OVERRIDE/CHOWN/FOWNER cover file-ownership fixups at startup;
	// NET_BIND_SERVICE lets a service bind low ports if it needs to. Notably
	// excluded: NET_RAW, NET_ADMIN, SYS_ADMIN, SYS_PTRACE, MKNOD, KILL. Manifest
	// service.cap_add is purely additive on top of this set, restricted to a
	// safe-cap allowlist validated at parse time.
	capAdd := []string{
		"CHOWN",
		"DAC_OVERRIDE",
		"FOWNER",
		"NET_BIND_SERVICE",
		"SETGID",
		"SETPCAP",
		"SETUID",
	}
	capAdd = append(capAdd, spec.CapAdd...)
	hc := &container.HostConfig{
		Mounts:        mounts,
		CapDrop:       []string{"ALL"},
		CapAdd:        capAdd,
		SecurityOpt:   []string{"no-new-privileges:true"},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		Resources: container.Resources{
			NanoCPUs: spec.NanoCPUs,
			Memory:   spec.MemoryBytes,
		},
	}
	nc := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {Aliases: spec.Aliases},
		},
	}

	resp, err := r.c.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
		Config:           cc,
		HostConfig:       hc,
		NetworkingConfig: nc,
		Name:             spec.Name,
	})
	return resp.ID, err
}

func (r *controllerClient) CreateContainerRaw(ctx context.Context, name string, cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig) (string, error) {
	resp, err := r.c.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: netCfg,
		Name:             name,
	})
	return resp.ID, err
}

func (r *controllerClient) StartContainer(ctx context.Context, id string) error {
	_, err := r.c.ContainerStart(ctx, id, dockerclient.ContainerStartOptions{})
	return err
}

func (r *controllerClient) StopContainer(ctx context.Context, id string) error {
	t := 10
	_, err := r.c.ContainerStop(ctx, id, dockerclient.ContainerStopOptions{Timeout: &t})
	return err
}

func (r *controllerClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	_, err := r.c.ContainerRemove(ctx, id, dockerclient.ContainerRemoveOptions{Force: force})
	return err
}

func (r *controllerClient) ContainerByName(ctx context.Context, name string) (string, string, error) {
	res, err := r.c.ContainerList(ctx, dockerclient.ContainerListOptions{
		All:     true,
		Filters: dockerclient.Filters{}.Add("name", name),
	})
	if err != nil {
		return "", "", err
	}
	for _, c := range res.Items {
		if slices.Contains(c.Names, "/"+name) {
			return c.ID, string(c.State), nil
		}
	}
	return "", "", nil
}

func (r *controllerClient) ListContainersByPrefix(ctx context.Context, prefix string) ([]ContainerSummary, error) {
	res, err := r.c.ContainerList(ctx, dockerclient.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := []ContainerSummary{}
	for _, c := range res.Items {
		for _, n := range c.Names {
			trimmed := strings.TrimPrefix(n, "/")
			if strings.HasPrefix(trimmed, prefix) {
				out = append(out, ContainerSummary{
					ID:     c.ID,
					Image:  c.Image,
					Name:   trimmed,
					State:  string(c.State),
					Status: c.Status,
					Labels: c.Labels,
				})
				break
			}
		}
	}
	return out, nil
}

func (r *controllerClient) HealthState(ctx context.Context, id string) (string, error) {
	res, err := r.c.ContainerInspect(ctx, id, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	if res.Container.State.Health == nil {
		return "", nil
	}
	return string(res.Container.State.Health.Status), nil
}

func (r *controllerClient) Logs(ctx context.Context, id string, tailLines int, follow bool, w io.Writer) error {
	opts := dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: follow}
	if tailLines > 0 {
		opts.Tail = strconv.Itoa(tailLines)
	}
	rc, err := r.c.ContainerLogs(ctx, id, opts)
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	return err
}

func (r *controllerClient) NetworkContainerCount(ctx context.Context, name string) (int, bool, error) {
	res, err := r.c.NetworkInspect(ctx, name, dockerclient.NetworkInspectOptions{})
	if err != nil {
		if isNotFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return len(res.Network.Containers), true, nil
}

func (r *controllerClient) ConnectNetwork(ctx context.Context, containerName, networkName string) error {
	_, err := r.c.NetworkConnect(ctx, networkName, dockerclient.NetworkConnectOptions{
		Container: containerName,
	})
	if err == nil {
		return nil
	}
	// Docker returns a conflict error when the container is already attached
	// to the network. Treat that as success so attach is idempotent.
	if msg := err.Error(); strings.Contains(msg, "already exists") || strings.Contains(msg, "is already attached") {
		return nil
	}
	return err
}
