package dockerx

import (
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/manifest"
)

func TestPlan_NetworkAndContainerNaming(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Services: map[string]manifest.Service{
			"neo4j": {Image: "neo4j:5.20", Volumes: map[string]string{"data": "/data"}},
		},
	}
	p := Plan("testproj", m)

	if p.NetworkName != "glovebox-stack-testproj" {
		t.Errorf("network = %q", p.NetworkName)
	}
	if !p.NetworkInternal {
		t.Errorf("network must be internal")
	}
	if len(p.Containers) != 1 {
		t.Fatalf("got %d containers", len(p.Containers))
	}
	c := p.Containers[0]
	if c.Name != "glovebox-stack-testproj-neo4j" {
		t.Errorf("name = %q", c.Name)
	}
	if len(c.Aliases) != 1 || c.Aliases[0] != "neo4j" {
		t.Errorf("alias = %v", c.Aliases)
	}
	if c.Image != "neo4j:5.20" {
		t.Errorf("image = %q", c.Image)
	}
	if len(c.Mounts) != 1 || c.Mounts[0].VolumeName != "glovebox-stack-testproj-neo4j-data" {
		t.Errorf("mounts = %+v", c.Mounts)
	}
}

func TestPlan_DefaultHealthcheckPerImage(t *testing.T) {
	cases := []struct {
		image     string
		mustMatch string // substring the probe must contain
	}{
		{"redis:7-alpine", "redis-cli"},
		{"docker.io/redis:8.0", "redis-cli"},
		{"docker.io/library/redis:8.0", "redis-cli"},
		{"postgres:16", "pg_isready"},
		{"mysql:8.4", "mysqladmin"},
		{"rabbitmq:3-management", "rabbitmq-diagnostics"},
		{"neo4j:5", "wget"},
		{"neo4j:5.20-enterprise", "7474"},
	}
	for _, c := range cases {
		t.Run(c.image, func(t *testing.T) {
			m := &manifest.Manifest{
				Version:  1,
				Services: map[string]manifest.Service{"svc": {Image: c.image}},
			}
			p := Plan("p1", m)
			hc := p.Containers[0].Healthcheck
			if hc == nil {
				t.Fatalf("default healthcheck not injected for %s", c.image)
			}
			if hc.Test[0] != "CMD-SHELL" || !strings.Contains(hc.Test[1], c.mustMatch) {
				t.Errorf("probe = %v, want substring %q", hc.Test, c.mustMatch)
			}
		})
	}
}

func TestPlan_NoDefaultHealthcheckForUnknownImage(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Services: map[string]manifest.Service{"x": {Image: "unknownimage:1.0"}},
	}
	p := Plan("p1", m)
	if p.Containers[0].Healthcheck != nil {
		t.Errorf("expected no default healthcheck, got %+v", p.Containers[0].Healthcheck)
	}
}

func TestPlan_PropagatesCapAdd(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Services: map[string]manifest.Service{
			"db": {Image: "neo4j:5.20", CapAdd: []string{"IPC_LOCK", "SYS_NICE"}},
		},
	}
	p := Plan("p1", m)
	if len(p.Containers) != 1 {
		t.Fatalf("got %d containers", len(p.Containers))
	}
	got := p.Containers[0].CapAdd
	if len(got) != 2 || got[0] != "IPC_LOCK" || got[1] != "SYS_NICE" {
		t.Errorf("cap_add = %v, want [IPC_LOCK SYS_NICE]", got)
	}
}

func TestPlan_ResolvesResourceLimits(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Services: map[string]manifest.Service{
			"redis": {
				Image:     "redis:7-alpine",
				Resources: &manifest.Resources{CPUs: "1.5", Memory: "512m"},
			},
		},
	}
	p := Plan("p1", m)
	got := p.Containers[0]
	if got.NanoCPUs != int64(1.5*1e9) {
		t.Errorf("NanoCPUs = %d, want %d", got.NanoCPUs, int64(1.5*1e9))
	}
	if got.MemoryBytes != 512<<20 {
		t.Errorf("MemoryBytes = %d, want %d", got.MemoryBytes, 512<<20)
	}
}

func TestPlan_ResourcesAbsentMeansNoLimits(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Services: map[string]manifest.Service{"redis": {Image: "redis:7-alpine"}},
	}
	p := Plan("p1", m)
	got := p.Containers[0]
	if got.NanoCPUs != 0 || got.MemoryBytes != 0 {
		t.Errorf("expected zero limits when resources absent, got NanoCPUs=%d MemoryBytes=%d", got.NanoCPUs, got.MemoryBytes)
	}
}

func TestPlan_ManifestHealthcheckWinsOverDefault(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Services: map[string]manifest.Service{
			"redis": {
				Image: "redis:7-alpine",
				Healthcheck: &manifest.Healthcheck{
					Test:     []string{"CMD", "echo", "ok"},
					Interval: "10s",
					Retries:  3,
					Timeout:  "1s",
				},
			},
		},
	}
	p := Plan("p1", m)
	hc := p.Containers[0].Healthcheck
	if hc == nil {
		t.Fatal("expected healthcheck from manifest")
	}
	if hc.Test[0] != "CMD" || hc.Test[1] != "echo" || hc.Test[2] != "ok" {
		t.Errorf("manifest healthcheck not used: %v", hc.Test)
	}
}
