package config

// GBX_* environment variable names. Centralized so producers (the host CLI,
// and stack.go which injects them into agent containers) and consumers read
// and write the same key - a renamed string in one place can't silently
// diverge from another.
const (
	EnvConfigDir          = "GBX_CONFIG_DIR"
	EnvStateDir           = "GBX_STATE_DIR"
	EnvLibexec            = "GBX_LIBEXEC"
	EnvControllerURL      = "GBX_CONTROLLER_URL"
	EnvControllerHostPort = "GBX_CONTROLLER_HOST_PORT"
	EnvProjectID          = "GBX_PROJECT_ID"
	EnvOverridePID        = "GBX_OVERRIDE_PID"
	EnvWaitTimeoutS       = "GBX_WAIT_TIMEOUT_S"
	EnvSkipStackUp        = "GBX_SKIP_STACK_UP"
	EnvAgentImage         = "GBX_AGENT_IMAGE"
	EnvTestMode           = "GBX_TEST_MODE"
)
