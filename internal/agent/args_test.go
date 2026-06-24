package agent_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/agent"
)

func defaultSpec() agent.CreateSpec {
	return agent.CreateSpec{
		PID:            "aaaa1111bbbb",
		Workspace:      "/work/foo",
		Image:          "glovebox-agent:local",
		StateProjDir:   "/state/projects/aaaa1111bbbb",
		StateSharedDir: "/state/shared",
		DockerDir:      "/libexec/docker",
		HostEnv:        map[string]string{},
	}
}

func envHas(t *testing.T, env []string, want string) {
	t.Helper()
	if !slices.Contains(env, want) {
		t.Fatalf("env missing %q in %v", want, env)
	}
}

func bindHas(t *testing.T, binds []string, want string) {
	t.Helper()
	for _, b := range binds {
		if b == want || strings.HasPrefix(b, want+":") {
			return
		}
	}
	t.Fatalf("binds missing %q in %v", want, binds)
}

func TestBuildCreateConfigBasicShape(t *testing.T) {
	cfg, hostCfg, netCfg, name := agent.BuildCreateConfig(defaultSpec())
	if name != "glovebox-agent-aaaa1111bbbb" {
		t.Fatalf("name = %q", name)
	}
	if cfg.User != agent.HostUser() {
		t.Fatalf("user = %q, want %q", cfg.User, agent.HostUser())
	}
	if cfg.WorkingDir != "/workspace" {
		t.Fatalf("workdir = %q", cfg.WorkingDir)
	}
	if cfg.Hostname != "glovebox-aaaa1111bbbb" {
		t.Fatalf("hostname = %q", cfg.Hostname)
	}
	if cfg.Image != "glovebox-agent:local" {
		t.Fatalf("image = %q", cfg.Image)
	}
	if !slices.Equal(cfg.Cmd, []string{"sleep", "infinity"}) {
		t.Fatalf("cmd = %v, want [sleep infinity]", cfg.Cmd)
	}
	if !slices.Contains(hostCfg.CapDrop, "ALL") {
		t.Fatalf("CapDrop missing ALL: %v", hostCfg.CapDrop)
	}
	if !slices.Contains(hostCfg.SecurityOpt, "no-new-privileges:true") {
		t.Fatalf("SecurityOpt missing no-new-privileges: %v", hostCfg.SecurityOpt)
	}
	if _, ok := netCfg.EndpointsConfig[agent.AgentNetwork]; !ok {
		t.Fatalf("EndpointsConfig missing %s: %v", agent.AgentNetwork, netCfg.EndpointsConfig)
	}
}

func TestBuildCreateConfigWorkspaceMount(t *testing.T) {
	spec := defaultSpec()
	spec.Workspace = "/work/custom"
	_, hostCfg, _, _ := agent.BuildCreateConfig(spec)
	bindHas(t, hostCfg.Binds, "/work/custom:/workspace")
}

func TestBuildCreateConfigForwardsHostEnv(t *testing.T) {
	spec := defaultSpec()
	spec.HostEnv = map[string]string{"ANTHROPIC_API_KEY": "sk-test"}
	cfg, _, _, _ := agent.BuildCreateConfig(spec)
	envHas(t, cfg.Env, "ANTHROPIC_API_KEY=sk-test")
}

func TestBuildCreateConfigEmptyHostEnvStillForwardsKey(t *testing.T) {
	spec := defaultSpec()
	cfg, _, _, _ := agent.BuildCreateConfig(spec)
	envHas(t, cfg.Env, "ANTHROPIC_API_KEY=")
}

func TestBuildCreateConfigExtraMountsRW(t *testing.T) {
	spec := defaultSpec()
	spec.Mounts = []agent.Mount{{Host: "/host/docs", Container: "/mnt/docs", Mode: "rw"}}
	_, hostCfg, _, _ := agent.BuildCreateConfig(spec)
	bindHas(t, hostCfg.Binds, "/host/docs:/mnt/docs")
	for _, b := range hostCfg.Binds {
		if b == "/host/docs:/mnt/docs:ro" {
			t.Fatalf("rw mount must not get :ro suffix: %v", hostCfg.Binds)
		}
	}
}

func TestBuildCreateConfigExtraMountsRO(t *testing.T) {
	spec := defaultSpec()
	spec.Mounts = []agent.Mount{{Host: "/host/lib", Container: "/mnt/lib", Mode: "ro"}}
	_, hostCfg, _, _ := agent.BuildCreateConfig(spec)
	bindHas(t, hostCfg.Binds, "/host/lib:/mnt/lib:ro")
}

func TestBuildCreateConfigExtraMountsAppearAfterFixedMounts(t *testing.T) {
	spec := defaultSpec()
	spec.Mounts = []agent.Mount{{Host: "/h", Container: "/c", Mode: "rw"}}
	_, hostCfg, _, _ := agent.BuildCreateConfig(spec)
	workspaceIdx, extraIdx := -1, -1
	for i, b := range hostCfg.Binds {
		if strings.HasPrefix(b, "/work/foo:/workspace") {
			workspaceIdx = i
		}
		if strings.HasPrefix(b, "/h:/c") {
			extraIdx = i
		}
	}
	if workspaceIdx == -1 || extraIdx == -1 || workspaceIdx >= extraIdx {
		t.Fatalf("extra mount must appear after fixed mounts: workspace=%d extra=%d binds=%v",
			workspaceIdx, extraIdx, hostCfg.Binds)
	}
}

func TestBuildCreateConfigStateDirVolumes(t *testing.T) {
	_, hostCfg, _, _ := agent.BuildCreateConfig(defaultSpec())
	for _, want := range []string{
		"/state/projects/aaaa1111bbbb/claude:/home/gbx/.claude",
		"/state/shared/npm:/home/gbx/.npm",
	} {
		bindHas(t, hostCfg.Binds, want)
	}
}
