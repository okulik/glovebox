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
		WorkingDir: WorkspaceDir,
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
		config.EnvProjectID:        spec.PID,
		config.EnvControllerURL:    agentControllerURL(),
		"AIDER_INPUT_HISTORY_FILE": HomeAider + "/.aider.input.history",
		"AIDER_CHAT_HISTORY_FILE":  HomeAider + "/.aider.chat.history.md",
		"UV_TOOL_DIR":              HomeUvTools,
		"UV_TOOL_BIN_DIR":          HomeLocalBin,
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
		spec.Workspace + ":" + WorkspaceDir,
		spec.DockerDir + "/../defaults/docker-sandbox.md:" + DockerSandboxDoc + ":ro",
		spec.DockerDir + "/../defaults/proxy-sandbox.md:" + ProxySandboxDoc + ":ro",
		spec.StateProjDir + "/claude:" + HomeClaude,
		spec.StateProjDir + "/claude:" + WorkspaceDir + "/.claude",
		spec.StateProjDir + "/claude/.claude.json:" + HomeClaudeJSON,
		spec.StateProjDir + "/codex:" + HomeCodex,
		spec.StateProjDir + "/aider:" + HomeAider,
		spec.StateProjDir + "/opencode:" + HomeOpencode,
		spec.StateProjDir + "/pi:" + HomePi,
		spec.StateProjDir + "/gemini:" + HomeGemini,
		spec.StateProjDir + "/hermes:" + HomeHermes,
		spec.StateSharedDir + "/npm:" + HomeNpm,
		spec.StateSharedDir + "/uv-tools:" + HomeUvTools,
		spec.StateSharedDir + "/bin:" + HomeLocalBin,
		spec.StateSharedDir + "/cache:" + HomeCache,
		spec.StateSharedDir + "/shell-history:" + HomeShellHist,
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
