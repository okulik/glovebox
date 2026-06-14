package config

import (
	"os"
	"strconv"
)

// GbxaConfig holds the env-derived knobs the in-container gbxa dispatcher
// reads. ControllerURL and ProjectID are typically set by the host (when
// agent.BuildCreateConfig wires up the agent container's env); WaitTimeout
// is an operator-tunable override for `gbx-stack wait`.
//
// GbxaConfig.ControllerURL's default differs from GbxConfig.ControllerURL:
// inside the container we reach the controller by DNS alias on the
// glovebox-internal network, not via the host's loopback port mapping.
type GbxaConfig struct {
	// ControllerURL is the in-network endpoint, default
	// http://stack-controller:7000.
	ControllerURL string
	// ProjectID scopes API calls to a single project. Defaults to "default"
	// so commands work in fresh single-project setups.
	ProjectID string
	// WaitTimeoutSeconds bounds `gbx-stack wait`. Default 1800.
	WaitTimeoutSeconds int
}

// GbxaFromEnv reads the GBX_* env vars consumed by the in-container gbxa
// dispatcher and applies defaults.
func GbxaFromEnv() GbxaConfig {
	timeout := 1800
	if v := os.Getenv("GBX_WAIT_TIMEOUT_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}
	return GbxaConfig{
		ControllerURL:      envOr("GBX_CONTROLLER_URL", "http://stack-controller:7000"),
		ProjectID:          envOr("GBX_PROJECT_ID", "default"),
		WaitTimeoutSeconds: timeout,
	}
}
