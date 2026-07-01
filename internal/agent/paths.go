package agent

// Container-side filesystem paths baked into every agent container.
const (
	// WorkspaceDir is the agent's working directory: the host workspace is
	// bind-mounted here and every agent runs with it as cwd.
	WorkspaceDir = "/workspace"

	// Home is the agent user's home directory inside the container.
	Home = "/home/gbx"

	// Sandbox instruction docs mounted read-only into the container.
	DockerSandboxDoc = "/etc/glovebox/docker-sandbox.md"
	ProxySandboxDoc  = "/etc/glovebox/proxy-sandbox.md"
)

// Per-agent state/cache directories under Home that glovebox bind-mounts from
// the host.
const (
	HomeClaude     = Home + "/.claude"
	HomeClaudeJSON = Home + "/.claude.json"
	HomeCodex      = Home + "/.codex"
	HomeAider      = Home + "/.aider"
	HomeOpencode   = Home + "/.local/share/opencode"
	HomePi         = Home + "/.pi"
	HomeGemini     = Home + "/.gemini"
	HomeHermes     = Home + "/.hermes"
	HomeNpm        = Home + "/.npm"
	HomeUvTools    = Home + "/.local/share/uv-tools"
	HomeLocalBin   = Home + "/.local/bin"
	HomeCache      = Home + "/.cache"
	HomeShellHist  = Home + "/.shell-history"
)
