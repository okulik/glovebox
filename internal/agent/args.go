// Package agent owns per-project agent container concerns: docker create-
// config construction, idempotent container lifecycle, and lazy per-agent
// binary install/update.
package agent

import (
	"fmt"
	"net"
	"os"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/okulik/glovebox/internal/config"
)

// CreateSpec holds the inputs needed to compute the per-project agent
// container's Docker API configs. All fields are required (zero values yield
// broken args). HostEnv may be an empty map but must not be nil. ExtraMounts
// may be nil.
//
//nolint:govet // fieldalignment
type CreateSpec struct {
	PID            string
	Workspace      string
	Image          string
	StateProjDir   string
	StateSharedDir string
	DockerDir      string // ${GBX_LIBEXEC}/docker - used to compute the defaults mount path
	HostEnv        map[string]string
	ExtraMounts    []Mount // appended after the fixed mounts; see ReadMounts.
	// Labels are applied to container.Config.Labels. The host CLI uses this
	// to set `io.glovebox.test=1` in test mode so the bash cleanup helper
	// can identify-and-remove test agents with a single `--filter label=...`.
	Labels map[string]string
}

// hostEnvKeys is the canonical list of host environment keys forwarded into
// the agent container.
var hostEnvKeys = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"OPENROUTER_API_KEY",
	"GOOGLE_API_KEY",
	"DEEPSEEK_API_KEY",
	"GROQ_API_KEY",
	"MISTRAL_API_KEY",
	"AWS_REGION",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"GOOGLE_APPLICATION_CREDENTIALS",
}

const (
	// AgentNetwork is the per-project agent's primary network - the same one
	// the egress proxy and stack-controller sit on, so the agent can reach
	// `http://proxy:3128` and the stack-controller's internal listener by alias.
	AgentNetwork = "glovebox-internal"
	// Default stack controller internal host name.
	StackControllerHost = "http://stack-controller"
)

// agentControllerURL is the in-network URL the agent uses to reach the
// stack-controller. The port follows CONTROLLER_INTERNAL_ADDR so an
// operator who overrides the controller's listener stays in sync with the
// agent's outgoing requests.
func agentControllerURL() string {
	addr := config.ControllerFromEnv().InternalAddr
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		// InternalAddr was malformed; fall back to the package default's port.
		_, port, _ = net.SplitHostPort(config.DefaultControllerInternalAddr)
	}
	return StackControllerHost + ":" + port
}

// BuildCreateConfig computes the typed Docker API configs for `docker create`
// of an agent container, plus the resolved container name.
func BuildCreateConfig(spec CreateSpec) (cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, name string) {
	name = "glovebox-agent-" + spec.PID

	env := []string{
		"HTTPS_PROXY=http://proxy:3128",
		"HTTP_PROXY=http://proxy:3128",
		"NO_PROXY=localhost,127.0.0.1,stack-controller",
		"GBX_PROJECT_ID=" + spec.PID,
		"GBX_CONTROLLER_URL=" + agentControllerURL(),
		"AIDER_INPUT_HISTORY_FILE=/home/gbx/.aider/.aider.input.history",
		"AIDER_CHAT_HISTORY_FILE=/home/gbx/.aider/.aider.chat.history.md",
		"UV_TOOL_DIR=/home/gbx/.local/share/uv-tools",
		"UV_TOOL_BIN_DIR=/home/gbx/.local/bin",
	}
	for _, key := range hostEnvKeys {
		env = append(env, fmt.Sprintf("%s=%s", key, spec.HostEnv[key]))
	}

	cfg = &container.Config{
		Image:      spec.Image,
		Hostname:   "glovebox-" + spec.PID,
		User:       HostUser(),
		WorkingDir: "/workspace",
		Cmd:        []string{"sleep", "infinity"},
		Env:        env,
		Labels:     spec.Labels,
	}

	binds := []string{
		spec.Workspace + ":/workspace",
		spec.DockerDir + "/../defaults/docker-sandbox.md:/etc/glovebox/docker-sandbox.md:ro",
		spec.DockerDir + "/../defaults/proxy-sandbox.md:/etc/glovebox/proxy-sandbox.md:ro",
		spec.StateProjDir + "/claude:/home/gbx/.claude",
		spec.StateProjDir + "/claude:/workspace/.claude",
		spec.StateProjDir + "/claude/.claude.json:/home/gbx/.claude.json",
		spec.StateProjDir + "/codex:/home/gbx/.codex",
		spec.StateProjDir + "/aider:/home/gbx/.aider",
		spec.StateProjDir + "/opencode:/home/gbx/.local/share/opencode",
		spec.StateProjDir + "/pi:/home/gbx/.pi",
		spec.StateProjDir + "/gemini:/home/gbx/.gemini",
		spec.StateProjDir + "/hermes:/home/gbx/.hermes",
		spec.StateSharedDir + "/npm:/home/gbx/.npm",
		spec.StateSharedDir + "/uv-tools:/home/gbx/.local/share/uv-tools",
		spec.StateSharedDir + "/bin:/home/gbx/.local/bin",
		spec.StateSharedDir + "/cache:/home/gbx/.cache",
		spec.StateSharedDir + "/shell-history:/home/gbx/.shell-history",
	}
	for _, m := range spec.ExtraMounts {
		v := fmt.Sprintf("%s:%s", m.Host, m.Container)
		if m.Mode == "ro" {
			v += ":ro"
		}
		binds = append(binds, v)
	}

	hostCfg = &container.HostConfig{
		Binds:         binds,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		CapDrop:       []string{"ALL"},
		SecurityOpt:   []string{"no-new-privileges:true"},
	}

	netCfg = &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			AgentNetwork: {},
		},
	}

	return cfg, hostCfg, netCfg, name
}

// HostUser formats the host uid:gid as a Docker `User` string for container
// create and exec.
func HostUser() string { return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()) }
