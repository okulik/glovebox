package config

import "testing"

func TestControllerFromEnvDefaults(t *testing.T) {
	t.Setenv("CONTROLLER_DOCKER_HOST", "")
	t.Setenv("CONTROLLER_STATE_DIR", "")
	t.Setenv("CONTROLLER_IMAGE_ALLOWLIST", "")
	t.Setenv("CONTROLLER_INTERNAL_ADDR", "")
	t.Setenv("CONTROLLER_HOST_ADDR", "")
	cfg := ControllerFromEnv()
	if cfg.DockerHost != "tcp://socket-proxy:2375" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.StateDir != "/state" {
		t.Errorf("StateDir = %q", cfg.StateDir)
	}
	if cfg.InternalAddr != ":7000" {
		t.Errorf("InternalAddr = %q", cfg.InternalAddr)
	}
	if cfg.HostAddr != ":7001" {
		t.Errorf("HostAddr = %q", cfg.HostAddr)
	}
}

func TestControllerFromEnvOverrides(t *testing.T) {
	t.Setenv("CONTROLLER_DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("CONTROLLER_INTERNAL_ADDR", ":9000")
	cfg := ControllerFromEnv()
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.InternalAddr != ":9000" {
		t.Errorf("InternalAddr = %q", cfg.InternalAddr)
	}
}
