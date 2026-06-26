package agent

import (
	"fmt"
	"net"
	"os"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/okulik/glovebox/internal/config"
)

type CreateSpec struct {
	PID            string
	Workspace      string
	Image          string
	StateProjDir   string
	StateSharedDir string
	DockerDir      string
	HostEnv        map[string]string
	Labels         map[string]string
	Mounts         []Mount
}

var HostEnvVars = []string{
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

func agentControllerURL() string {
	_, port, err := net.SplitHostPort(config.ControllerFromEnv().InternalAddr)
	if err != nil || port == "" {
		_, port, _ = net.SplitHostPort(config.DefaultControllerInternalAddr)
	}
	return config.StackControllerHost + ":" + port
}

func BuildCreateConfig(spec CreateSpec) (cfg *container.Config, hostCfg *container.HostConfig, netCfg *network.NetworkingConfig, containerName string) {
	hostCfg = &container.HostConfig{
		Binds:         buildBinds(spec),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		CapDrop:       []string{"ALL"},
		SecurityOpt:   []string{"no-new-privileges:true"},
	}

	netCfg = &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			config.NetworkInternal: {},
		},
	}

	cfg = &container.Config{
		Image:      spec.Image,
		Hostname:   config.ContainerPrefix + spec.PID,
		User:       HostUser(),
		WorkingDir: "/workspace",
		Cmd:        []string{"sleep", "infinity"},
		Env:        buildEnvVars(spec),
		Labels:     spec.Labels,
	}

	containerName = config.ContainerAgentPrefix + spec.PID

	return
}

// HostUser formats the host uid:gid as a Docker `User` string for container
// create and exec.
func HostUser() string { return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()) }

func buildEnvVars(spec CreateSpec) []string {
	envVars := map[string]string{
		"HTTPS_PROXY":              config.ProxyURL,
		"HTTP_PROXY":               config.ProxyURL,
		"NO_PROXY":                 "localhost,127.0.0.1," + config.HostnameStackController,
		"GBX_PROJECT_ID":           spec.PID,
		"GBX_CONTROLLER_URL":       agentControllerURL(),
		"AIDER_INPUT_HISTORY_FILE": "/home/gbx/.aider/.aider.input.history",
		"AIDER_CHAT_HISTORY_FILE":  "/home/gbx/.aider/.aider.chat.history.md",
		"UV_TOOL_DIR":              "/home/gbx/.local/share/uv-tools",
		"UV_TOOL_BIN_DIR":          "/home/gbx/.local/bin",
	}

	// host env vars overwrite internal ones
	for _, key := range HostEnvVars {
		envVars[key] = spec.HostEnv[key]
	}

	env := make([]string, 0, len(envVars))
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

func buildBinds(spec CreateSpec) []string {
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

	// bind additional mounts (added through gbx mount command)
	for _, m := range spec.Mounts {
		v := fmt.Sprintf("%s:%s", m.Host, m.Container)
		if m.Mode == "ro" {
			v += ":ro"
		}
		binds = append(binds, v)
	}

	return binds
}
