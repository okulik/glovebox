package manifest

// Manifest is the top-level structure of .glovebox/stack.yml.
type Manifest struct {
	Services map[string]Service `yaml:"services" validate:"required,min=1,max=16,dive"`
	Version  int                `yaml:"version"  validate:"eq=1"`
}

// Service describes one container declared in the manifest.
type Service struct {
	Image       string            `yaml:"image"       validate:"required"`
	Env         map[string]string `yaml:"env,omitempty"`
	Volumes     map[string]string `yaml:"volumes,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	Resources   *Resources        `yaml:"resources,omitempty"`
	// CapAdd lists Linux capabilities to grant on top of the always-on minimum
	// set. Each entry is validated against Rules.AllowedCaps (safe-cap allowlist).
	CapAdd []string `yaml:"cap_add,omitempty"`
}

// Healthcheck mirrors Compose's healthcheck shape.
type Healthcheck struct {
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Test     []string `yaml:"test"`
	Retries  int      `yaml:"retries,omitempty"`
}

// Resources captures bounded CPU and memory caps.
type Resources struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}
