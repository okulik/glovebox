package agent

// Names is the canonical, ordered set of agent/harness names glovebox supports.
// It is the single source of truth for "which agents exist": per-project state
// subdirs (container.go), the `gbx state-size` listing, the conversation-export
// registry (internal/convexport), and help text all key off this list. The
// name-keyed data maps that carry per-agent detail - installSpecs here and the
// gbxa dispatch table - are asserted against it by tests so they can't drift.
//
// Order is the install order used across the codebase (claude first, then the
// npm agents, then the uv agents); it's user-visible in listings, so keep it
// stable.
var Names = []string{"claude", "codex", "opencode", "pi", "gemini", "aider", "hermes"}

// Supported reports whether name is one of the known agents.
func Supported(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}
