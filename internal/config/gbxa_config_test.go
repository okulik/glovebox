package config_test

import (
	"testing"

	"github.com/okulik/glovebox/internal/config"
)

func TestGbxaFromEnvDefaults(t *testing.T) {
	t.Setenv(config.EnvControllerURL, "")
	t.Setenv(config.EnvProjectID, "")
	t.Setenv(config.EnvWaitTimeoutS, "")
	cfg := config.GbxaFromEnv()
	if cfg.ControllerURL != "http://stack-controller:7000" {
		t.Errorf("ControllerURL = %q", cfg.ControllerURL)
	}
	if cfg.ProjectID != config.DefaultProjectID {
		t.Errorf("ProjectID = %q", cfg.ProjectID)
	}
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("WaitTimeoutSeconds = %d", cfg.WaitTimeoutSeconds)
	}
}

func TestGbxaFromEnvOverrides(t *testing.T) {
	t.Setenv(config.EnvControllerURL, "http://controller.test:7000")
	t.Setenv(config.EnvProjectID, "myproj")
	t.Setenv(config.EnvWaitTimeoutS, "60")
	cfg := config.GbxaFromEnv()
	if cfg.ControllerURL != "http://controller.test:7000" {
		t.Errorf("ControllerURL = %q", cfg.ControllerURL)
	}
	if cfg.ProjectID != "myproj" {
		t.Errorf("ProjectID = %q", cfg.ProjectID)
	}
	if cfg.WaitTimeoutSeconds != 60 {
		t.Errorf("WaitTimeoutSeconds = %d", cfg.WaitTimeoutSeconds)
	}
}

func TestGbxaFromEnvIgnoresInvalidTimeout(t *testing.T) {
	t.Setenv(config.EnvWaitTimeoutS, "not-a-number")
	cfg := config.GbxaFromEnv()
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("invalid timeout should fall back to default, got %d", cfg.WaitTimeoutSeconds)
	}
}

func TestGbxaFromEnvIgnoresNegativeTimeout(t *testing.T) {
	t.Setenv(config.EnvWaitTimeoutS, "-5")
	cfg := config.GbxaFromEnv()
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("negative timeout should fall back to default, got %d", cfg.WaitTimeoutSeconds)
	}
}
