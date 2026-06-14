// Package config is the typed home for every process's environment-variable
// contract: ControllerConfig for the stack-controller process, GbxConfig for
// the host CLI, GbxaConfig for the in-container dispatcher. Each binary
// reads its own struct via the matching `XxxFromEnv` constructor.
package config

import "os"

// ControllerConfig holds runtime configuration for the stack-controller
// process. It is consumed by cmd/controller and produced on the host side
// by internal/stack (which translates these values into env vars on the
// controller container at create time).
type ControllerConfig struct {
	// DockerHost is the URL of the docker-socket-proxy
	// (e.g. "tcp://socket-proxy:2375").
	DockerHost string
	// StateDir is where projects.json and other persistent controller state lives.
	StateDir string
	// ImageAllowlistPath is the file containing one registry per line.
	ImageAllowlistPath string
	// InternalAddr is the HTTP listener for agent traffic, e.g. ":7000".
	InternalAddr string
	// HostAddr is the HTTP listener for host-CLI traffic, e.g. ":7001"
	// (bound to host loopback by internal/stack's port mapping).
	HostAddr string
}

// ControllerFromEnv reads the CONTROLLER_* env vars and applies defaults.
func ControllerFromEnv() ControllerConfig {
	return ControllerConfig{
		DockerHost:         envOr("CONTROLLER_DOCKER_HOST", "tcp://socket-proxy:2375"),
		StateDir:           envOr("CONTROLLER_STATE_DIR", "/state"),
		ImageAllowlistPath: envOr("CONTROLLER_IMAGE_ALLOWLIST", "/config/image-allowlist.txt"),
		InternalAddr:       envOr("CONTROLLER_INTERNAL_ADDR", ":7000"),
		HostAddr:           envOr("CONTROLLER_HOST_ADDR", ":7001"),
	}
}

// envOr is the shared "env-or-default" helper used by every XxxFromEnv
// constructor in this package.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
