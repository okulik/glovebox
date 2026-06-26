package dockerx

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

// FakeContainer is the recorded state of one fake container.
//
//nolint:govet // fieldalignment: test fake; readability over byte packing.
type FakeContainer struct {
	ID     string
	Image  string
	State  string
	Health string
	Status string
	Labels map[string]string
}

// Fake is an in-memory Client for tests. It records every call so tests
// can assert on what the apply path actually invoked.
type Fake struct {
	Networks   map[string]bool
	Volumes    map[string]bool
	Containers map[string]FakeContainer
	PullErr    map[string]error
	CreateErr  map[string]error
	HealthSeq  map[string][]string
	// NetworkContainers maps network name → container count. Presence in
	// the map means the network exists (count may be 0); absence means the
	// network is absent, which mirrors NetworkInspect returning NotFound.
	NetworkContainers map[string]int
	PulledImages      []string
	NetworkConnects   []struct{ Container, Network string }
	mu                sync.Mutex
}

func NewFake() *Fake {
	return &Fake{
		Networks:          map[string]bool{},
		Volumes:           map[string]bool{},
		Containers:        map[string]FakeContainer{},
		PullErr:           map[string]error{},
		CreateErr:         map[string]error{},
		HealthSeq:         map[string][]string{},
		NetworkContainers: map[string]int{},
	}
}

func (f *Fake) EnsureNetwork(_ context.Context, name string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Networks[name] = true
	return nil
}

func (f *Fake) RemoveNetwork(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Networks, name)
	return nil
}

func (f *Fake) PullImage(_ context.Context, image string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.PullErr[image]; ok {
		return e
	}
	f.PulledImages = append(f.PulledImages, image)
	return nil
}

func (f *Fake) PullImageStream(_ context.Context, image string, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.PullErr[image]; ok {
		return e
	}
	f.PulledImages = append(f.PulledImages, image)
	return nil
}

func (f *Fake) EnsureVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Volumes[name] = true
	return nil
}

func (f *Fake) RemoveVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Volumes, name)
	return nil
}

func (f *Fake) ListVolumesByPrefix(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for name := range f.Volumes {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out, nil
}

func (f *Fake) CreateContainer(_ context.Context, spec ContainerSpec, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.CreateErr[spec.Name]; ok {
		return "", e
	}
	id := "id-" + spec.Name
	f.Containers[spec.Name] = FakeContainer{ID: id, Image: spec.Image, State: string(container.StateCreated), Health: "starting"}
	return id, nil
}

func (f *Fake) CreateContainerRaw(_ context.Context, name string, cfg *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.CreateErr[name]; ok {
		return "", e
	}
	id := "id-" + name
	image := ""
	if cfg != nil {
		image = cfg.Image
	}
	f.Containers[name] = FakeContainer{ID: id, Image: image, State: string(container.StateCreated)}
	return id, nil
}

func (f *Fake) StartContainer(_ context.Context, idOrName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.Containers[idOrName]; ok {
		c.State = string(container.StateRunning)
		f.Containers[idOrName] = c
		return nil
	}
	for name, c := range f.Containers {
		if c.ID == idOrName {
			c.State = string(container.StateRunning)
			f.Containers[name] = c
			return nil
		}
	}
	return errors.New("not found")
}

func (f *Fake) StopContainer(_ context.Context, idOrName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.Containers[idOrName]; ok {
		c.State = string(container.StateExited)
		f.Containers[idOrName] = c
		return nil
	}
	for name, c := range f.Containers {
		if c.ID == idOrName {
			c.State = string(container.StateExited)
			f.Containers[name] = c
			return nil
		}
	}
	return nil
}

func (f *Fake) RemoveContainer(_ context.Context, idOrName string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Containers are keyed by name, so a name lookup is the cheap path. Fall
	// through to an ID scan to match the real Docker API which accepts either.
	if _, ok := f.Containers[idOrName]; ok {
		delete(f.Containers, idOrName)
		return nil
	}
	for name, c := range f.Containers {
		if c.ID == idOrName {
			delete(f.Containers, name)
			return nil
		}
	}
	return nil
}

func (f *Fake) ContainerByName(_ context.Context, name string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.Containers[name]
	if !ok {
		return "", "", nil
	}
	return c.ID, c.State, nil
}

func (f *Fake) ListContainersByPrefix(_ context.Context, prefix string) ([]ContainerSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []ContainerSummary{}
	for name, c := range f.Containers {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			out = append(out, ContainerSummary{
				ID:     c.ID,
				Image:  c.Image,
				Name:   name,
				State:  c.State,
				Status: c.Status,
				Labels: c.Labels,
			})
		}
	}
	return out, nil
}

func (f *Fake) HealthState(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, c := range f.Containers {
		if c.ID == id {
			if seq, ok := f.HealthSeq[id]; ok && len(seq) > 0 {
				c.Health = seq[0]
				f.HealthSeq[id] = seq[1:]
				f.Containers[name] = c
			}
			return c.Health, nil
		}
	}
	return "", nil
}

func (f *Fake) Logs(_ context.Context, _ string, _ int, _ bool, _ io.Writer) error { return nil }

func (f *Fake) NetworkContainerCount(_ context.Context, name string) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.NetworkContainers[name]; ok {
		return n, true, nil
	}
	return 0, false, nil
}

func (f *Fake) ConnectNetwork(_ context.Context, containerName, networkName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.NetworkConnects = append(f.NetworkConnects, struct{ Container, Network string }{containerName, networkName})
	return nil
}
