package config

import (
	"os"
	"path/filepath"
)

const (
	ActiveProjectPath = "active-project"
	PluginsPath       = "plugins"
	ProjectsPath      = "projects"
	StatePath         = "state"
	WorkspacePath     = "workspace-path"

	NetworkInternal = "glovebox-internal"
	NetworkControl  = "glovebox-control"
	NetworkEgress   = "glovebox-egress"

	ContainerAgentPrefix     = "glovebox-agent-"
	ContainerStackPrefix     = "glovebox-stack-"
	ContainerEgressProxy     = "glovebox-egress-proxy"
	ContainerSocketProxy     = "glovebox-socket-proxy"
	ContainerStackController = "glovebox-stack-controller"

	ImageEgressProxy = "ubuntu/squid:latest"
	ImageSocketProxy = "tecnativa/docker-socket-proxy:0.3.0"
	ImageController  = "glovebox-stack-controller:local"

	StackControllerHost = "http://stack-controller"
)

// GbxConfig holds runtime configuration for the host CLI (cmd/gbx). It is
// the host-side mirror of ControllerConfig: same shape, different scope.
type GbxConfig struct {
	// Libexec is the read-only package dir (docker recipes, defaults). When
	// unset it stays empty; callers that need it check explicitly so error
	// messages name the variable rather than failing deep in the stack.
	Libexec string
	// ConfigDir is the user-writable state dir, default ~/.config/glovebox.
	ConfigDir string
	// StateDir defaults to ${ConfigDir}/state.
	StateDir string
	// ControllerHostPort is the host loopback port published from the
	// stack-controller container's HostAddr (default "17001"). The literal
	// "17001" avoids the macOS AFS3 conflict on the controller's container
	// port (7001).
	ControllerHostPort string
	// ControllerURL is what `gbx stack ...` posts to. Default is
	// http://127.0.0.1:${ControllerHostPort}.
	ControllerURL string
	// AgentImage is the repository:tag every project's agent container is
	// built from and run against. Tests can point this at a throwaway tag
	// so a `gbx rebuild` from one test doesn't untag the operator's real
	// glovebox-agent:local image.
	AgentImage string
	// TestMode is true when the host CLI runs from the test suite
	// (GBX_TEST_MODE=1). It is the single signal agent containers carry
	// the `io.glovebox.test=1` label so `cleanup_glovebox_test_resources`
	// can wipe leaks deterministically regardless of state-dir state.
	TestMode bool
}

// GbxFromEnv reads the GBX_* env vars consumed by the host CLI and applies
// defaults. Never errors - "missing required" checks belong to the call
// sites that actually need the field (notably GBX_LIBEXEC).
func GbxFromEnv() GbxConfig {
	home, _ := os.UserHomeDir()
	cfgDir := envOr("GBX_CONFIG_DIR", filepath.Join(home, ".config", "glovebox"))
	stateDir := envOr("GBX_STATE_DIR", filepath.Join(cfgDir, "state"))
	port := envOr("GBX_CONTROLLER_HOST_PORT", "17001")
	return GbxConfig{
		Libexec:            os.Getenv("GBX_LIBEXEC"),
		ConfigDir:          cfgDir,
		StateDir:           stateDir,
		ControllerHostPort: port,
		ControllerURL:      envOr("GBX_CONTROLLER_URL", "http://127.0.0.1:"+port),
		AgentImage:         envOr("GBX_AGENT_IMAGE", "glovebox-agent:local"),
		TestMode:           os.Getenv("GBX_TEST_MODE") == "1",
	}
}
