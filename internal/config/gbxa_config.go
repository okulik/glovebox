package config

import (
	"os"
	"strconv"
)

const (
	DefaultProjectID = "default"
)

// GbxaConfig holds the env-derived knobs the in-container gbxa dispatcher
// reads.
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
	if v := os.Getenv(EnvWaitTimeoutS); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}
	return GbxaConfig{
		ControllerURL:      envOr(EnvControllerURL, "http://stack-controller:7000"),
		ProjectID:          envOr(EnvProjectID, DefaultProjectID),
		WaitTimeoutSeconds: timeout,
	}
}
