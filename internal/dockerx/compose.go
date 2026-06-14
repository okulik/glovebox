package dockerx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/okulik/glovebox/internal/manifest"
)

// StackPlan is the planned set of Docker resources for one project.
type StackPlan struct {
	ProjectID       string
	NetworkName     string
	Containers      []ContainerSpec
	Volumes         []string
	NetworkInternal bool
}

// ContainerSpec is the subset of container config we need to create.
type ContainerSpec struct {
	Env         map[string]string
	Healthcheck *Healthcheck
	Name        string
	Image       string
	Aliases     []string
	Mounts      []MountSpec
	CapAdd      []string
	NanoCPUs    int64
	MemoryBytes int64
}

// MountSpec is a named-volume mount.
type MountSpec struct {
	VolumeName string
	Target     string
}

// Healthcheck is the resolved container healthcheck.
type Healthcheck struct {
	Test     []string
	Interval time.Duration
	Retries  int
	Timeout  time.Duration
}

// Plan returns the planned resources for the given (projectID, manifest).
// Services (and their volumes) are emitted in sorted order so the output is
// deterministic - important for stable test fixtures and reproducible
// last_apply records.
func Plan(projectID string, m *manifest.Manifest) StackPlan {
	p := StackPlan{
		ProjectID:       projectID,
		NetworkName:     "glovebox-stack-" + projectID,
		NetworkInternal: true,
	}
	names := make([]string, 0, len(m.Services))
	for name := range m.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := m.Services[name]
		cs := ContainerSpec{
			Name:    fmt.Sprintf("glovebox-stack-%s-%s", projectID, name),
			Image:   svc.Image,
			Env:     svc.Env,
			Aliases: []string{name},
			CapAdd:  svc.CapAdd,
		}
		if svc.Resources != nil {
			if svc.Resources.CPUs != "" {
				if n, err := strconv.ParseFloat(svc.Resources.CPUs, 64); err == nil {
					cs.NanoCPUs = int64(n * 1e9)
				}
			}
			if b, err := manifest.ParseMemoryBytes(svc.Resources.Memory); err == nil {
				cs.MemoryBytes = b
			}
		}
		vnames := make([]string, 0, len(svc.Volumes))
		for vname := range svc.Volumes {
			vnames = append(vnames, vname)
		}
		sort.Strings(vnames)
		for _, vname := range vnames {
			target := svc.Volumes[vname]
			full := fmt.Sprintf("glovebox-stack-%s-%s-%s", projectID, name, vname)
			cs.Mounts = append(cs.Mounts, MountSpec{VolumeName: full, Target: target})
			p.Volumes = append(p.Volumes, full)
		}
		cs.Healthcheck = resolveHealthcheck(&svc)
		p.Containers = append(p.Containers, cs)
	}
	return p
}

// ServiceDefaults bundles the per-image knowledge the controller injects
// when a manifest doesn't supply it. Probe is the healthcheck command tuned
// to that image's official build (uses a tool guaranteed to be in the image
// - e.g. redis-cli in redis); Port is the canonical service port surfaced
// through GET /projects/{pid}/info so agents can discover it.
type ServiceDefaults struct {
	Probe []string
	Port  int
}

// serviceDefaults is the single source of truth for per-image defaults.
// Adding a service means editing one entry here - both the healthcheck
// injection in resolveHealthcheck() and the /info port lookup
// (api.DefaultPortFor) read from it.
var serviceDefaults = map[string]ServiceDefaults{
	"redis":    {Probe: []string{"CMD-SHELL", "redis-cli ping | grep -q PONG"}, Port: 6379},
	"postgres": {Probe: []string{"CMD-SHELL", "pg_isready -h 127.0.0.1 -q"}, Port: 5432},
	"mysql":    {Probe: []string{"CMD-SHELL", "mysqladmin ping -h 127.0.0.1 --silent"}, Port: 3306},
	"rabbitmq": {Probe: []string{"CMD-SHELL", "rabbitmq-diagnostics -q ping"}, Port: 5672},
	"neo4j":    {Probe: []string{"CMD-SHELL", "wget --quiet --tries=1 --spider http://localhost:7474/ || exit 1"}, Port: 7687},
}

// matchImagePrefix returns the entry whose key matches the leading "name"
// segment of img (either "name:tag" or "registry/.../name:tag"), or ok=false.
func matchImagePrefix(img string) (ServiceDefaults, bool) {
	for k, d := range serviceDefaults {
		if strings.HasPrefix(img, k+":") || strings.Contains(img, "/"+k+":") {
			return d, true
		}
	}
	return ServiceDefaults{}, false
}

// DefaultPortFor returns the canonical port for img, or 0 if no default is
// known. Exported so api/services.go can surface it via /info without
// duplicating the lookup table.
func DefaultPortFor(img string) int {
	if d, ok := matchImagePrefix(img); ok {
		return d.Port
	}
	return 0
}

func defaultHealthFor(image string) *Healthcheck {
	d, ok := matchImagePrefix(image)
	if !ok {
		return nil
	}
	return &Healthcheck{
		Test:     d.Probe,
		Interval: 2 * time.Second,
		Retries:  30,
		Timeout:  2 * time.Second,
	}
}

func resolveHealthcheck(svc *manifest.Service) *Healthcheck {
	if svc.Healthcheck != nil {
		// Manifest specified one - translate it.
		out := &Healthcheck{
			Test:    svc.Healthcheck.Test,
			Retries: svc.Healthcheck.Retries,
		}
		if d, err := time.ParseDuration(svc.Healthcheck.Interval); err == nil {
			out.Interval = d
		}
		if d, err := time.ParseDuration(svc.Healthcheck.Timeout); err == nil {
			out.Timeout = d
		}
		return out
	}
	// Otherwise try to synthesize one from the image name.
	return defaultHealthFor(svc.Image)
}
