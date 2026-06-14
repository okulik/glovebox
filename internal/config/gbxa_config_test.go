package config

import "testing"

func TestGbxaFromEnvDefaults(t *testing.T) {
	t.Setenv("GBX_CONTROLLER_URL", "")
	t.Setenv("GBX_PROJECT_ID", "")
	t.Setenv("GBX_WAIT_TIMEOUT_S", "")
	cfg := GbxaFromEnv()
	if cfg.ControllerURL != "http://stack-controller:7000" {
		t.Errorf("ControllerURL = %q", cfg.ControllerURL)
	}
	if cfg.ProjectID != "default" {
		t.Errorf("ProjectID = %q", cfg.ProjectID)
	}
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("WaitTimeoutSeconds = %d", cfg.WaitTimeoutSeconds)
	}
}

func TestGbxaFromEnvOverrides(t *testing.T) {
	t.Setenv("GBX_CONTROLLER_URL", "http://controller.test:7000")
	t.Setenv("GBX_PROJECT_ID", "myproj")
	t.Setenv("GBX_WAIT_TIMEOUT_S", "60")
	cfg := GbxaFromEnv()
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
	t.Setenv("GBX_WAIT_TIMEOUT_S", "not-a-number")
	cfg := GbxaFromEnv()
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("invalid timeout should fall back to default, got %d", cfg.WaitTimeoutSeconds)
	}
}

func TestGbxaFromEnvIgnoresNegativeTimeout(t *testing.T) {
	t.Setenv("GBX_WAIT_TIMEOUT_S", "-5")
	cfg := GbxaFromEnv()
	if cfg.WaitTimeoutSeconds != 1800 {
		t.Errorf("negative timeout should fall back to default, got %d", cfg.WaitTimeoutSeconds)
	}
}
