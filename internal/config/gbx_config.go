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
	SharedPath        = "shared"

	// DockerDirName is the recipes subdir under <libexec> holding the
	// Dockerfile, squid.conf, image-allowlist.txt, and friends.
	DockerDirName = "docker"

	NetworkInternal = "glovebox-internal"
	NetworkControl  = "glovebox-control"
	NetworkEgress   = "glovebox-egress"

	// ContainerPrefix is the common prefix on every container glovebox owns;
	// the role-specific prefixes and singleton names derive from it.
	ContainerPrefix          = "glovebox-"
	ContainerAgentPrefix     = ContainerPrefix + "agent-"
	ContainerStackPrefix     = ContainerPrefix + "stack-"
	ContainerEgressProxy     = ContainerPrefix + "egress-proxy"
	ContainerSocketProxy     = ContainerPrefix + "socket-proxy"
	ContainerStackController = ContainerPrefix + "stack-controller"

	// Hostnames are the in-network DNS names (and aliases) the singleton
	// stack containers answer to.
	HostnameSocketProxy     = "socket-proxy"
	HostnameProxy           = "proxy"
	HostnameStackController = "stack-controller"

	// ProxyURL is the egress proxy endpoint injected as HTTP(S)_PROXY into
	// every agent container.
	ProxyPort = "3128"
	ProxyURL  = "http://" + HostnameProxy + ":" + ProxyPort

	ImageEgressProxy = "ubuntu/squid:latest"
	ImageSocketProxy = "tecnativa/docker-socket-proxy:0.3.0"
	ImageController  = "glovebox-stack-controller:local"

	StackControllerHost = "http://" + HostnameStackController

	// Docker label keys glovebox stamps on its images/containers.
	LabelTest         = "io.glovebox.test"
	LabelImageCreated = "io.glovebox.image.created"

	// HealthPath is the controller's liveness endpoint.
	HealthPath = "/health"
)

// GbxConfig holds runtime configuration for the host CLI (cmd/gbx).
type GbxConfig struct {
	// Libexec is the read-only package dir (docker recipes, defaults).
	Libexec string
	// ConfigDir is the user-writable state dir, default ~/.config/glovebox.
	ConfigDir string
	// StateDir defaults to ${ConfigDir}/state.
	StateDir string
	// ControllerHostPort is the host loopback port published from the
	// stack-controller container's HostAddr (default "17001").
	ControllerHostPort string
	// ControllerURL is what `gbx stack ...` posts to. Default is
	// http://127.0.0.1:${ControllerHostPort}.
	ControllerURL string
	// AgentImage is the repository:tag every project's agent container is
	// built from and run against.
	AgentImage string
	// TestMode is true when the host CLI runs from the test suite
	// (GBX_TEST_MODE=1).
	TestMode bool
}

// GbxFromEnv reads the GBX_* env vars consumed by the host CLI and applies
// defaults. Never errors - "missing required" checks belong to the call
// sites that actually need the field (notably GBX_LIBEXEC).
func GbxFromEnv() GbxConfig {
	home, _ := os.UserHomeDir()
	cfgDir := envOr(EnvConfigDir, filepath.Join(home, ".config", "glovebox"))
	stateDir := envOr(EnvStateDir, filepath.Join(cfgDir, "state"))
	port := envOr(EnvControllerHostPort, "17001")
	return GbxConfig{
		Libexec:            os.Getenv(EnvLibexec),
		ConfigDir:          cfgDir,
		StateDir:           stateDir,
		ControllerHostPort: port,
		ControllerURL:      envOr(EnvControllerURL, "http://127.0.0.1:"+port),
		AgentImage:         envOr(EnvAgentImage, "glovebox-agent:local"),
		TestMode:           os.Getenv(EnvTestMode) == "1",
	}
}
