package config

import (
	"path/filepath"
	"testing"
)

func TestGbxFromEnvDefaults(t *testing.T) {
	t.Setenv(EnvLibexec, "")
	t.Setenv(EnvConfigDir, "")
	t.Setenv(EnvStateDir, "")
	t.Setenv(EnvControllerHostPort, "")
	t.Setenv(EnvControllerURL, "")
	t.Setenv("HOME", "/tmp/gbxconfig-test-home")
	cfg := GbxFromEnv()
	if cfg.Libexec != "" {
		t.Errorf("Libexec should be empty when unset, got %q", cfg.Libexec)
	}
	if cfg.ConfigDir != filepath.Join("/tmp/gbxconfig-test-home", ".config", "glovebox") {
		t.Errorf("ConfigDir = %q", cfg.ConfigDir)
	}
	if cfg.StateDir != filepath.Join(cfg.ConfigDir, "state") {
		t.Errorf("StateDir = %q, want under ConfigDir", cfg.StateDir)
	}
	if cfg.ControllerHostPort != "17001" {
		t.Errorf("ControllerHostPort = %q", cfg.ControllerHostPort)
	}
	if cfg.ControllerURL != "http://127.0.0.1:17001" {
		t.Errorf("ControllerURL = %q", cfg.ControllerURL)
	}
	if cfg.AgentImage != "glovebox-agent:local" {
		t.Errorf("AgentImage = %q", cfg.AgentImage)
	}
}

func TestGbxFromEnvAgentImageOverride(t *testing.T) {
	t.Setenv(EnvAgentImage, "glovebox-agent-test-aad:local")
	cfg := GbxFromEnv()
	if cfg.AgentImage != "glovebox-agent-test-aad:local" {
		t.Errorf("AgentImage override ignored: %q", cfg.AgentImage)
	}
}

func TestGbxFromEnvOverrides(t *testing.T) {
	t.Setenv(EnvLibexec, "/opt/glovebox/libexec")
	t.Setenv(EnvConfigDir, "/var/glovebox")
	t.Setenv(EnvStateDir, "/var/glovebox-state")
	t.Setenv(EnvControllerHostPort, "27001")
	cfg := GbxFromEnv()
	if cfg.Libexec != "/opt/glovebox/libexec" {
		t.Errorf("Libexec = %q", cfg.Libexec)
	}
	if cfg.ConfigDir != "/var/glovebox" {
		t.Errorf("ConfigDir = %q", cfg.ConfigDir)
	}
	if cfg.StateDir != "/var/glovebox-state" {
		t.Errorf("StateDir = %q", cfg.StateDir)
	}
	if cfg.ControllerHostPort != "27001" {
		t.Errorf("ControllerHostPort = %q", cfg.ControllerHostPort)
	}
	if cfg.ControllerURL != "http://127.0.0.1:27001" {
		t.Errorf("ControllerURL = %q", cfg.ControllerURL)
	}
}

func TestGbxFromEnvControllerURLOverride(t *testing.T) {
	t.Setenv(EnvControllerHostPort, "27001")
	t.Setenv(EnvControllerURL, "http://gbx-controller.example:8080")
	cfg := GbxFromEnv()
	if cfg.ControllerURL != "http://gbx-controller.example:8080" {
		t.Errorf("explicit GBX_CONTROLLER_URL must win: %q", cfg.ControllerURL)
	}
}
