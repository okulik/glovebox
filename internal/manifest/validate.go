package manifest

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Rules bundles the policy knobs the validator consults.
type Rules struct {
	AllowedRegistries map[string]struct{}
	AllowedEnvVars    map[string]struct{}
	// AllowedCaps is the safe-cap allowlist for service.cap_add. Entries must
	// match exactly (case-sensitive, no wildcards).
	AllowedCaps    map[string]struct{}
	MaxCPUs        float64
	MaxMemoryBytes int64
}

// defaultAllowedCaps is the safe-cap allowlist. Capabilities listed here are
// considered safe to grant on top of the always-on minimum set. Capabilities
// notably excluded (and which would require an explicit policy change): NET_RAW,
// NET_ADMIN, SYS_ADMIN, SYS_PTRACE, SYS_MODULE, SYS_RAWIO, SYS_TIME, SYS_BOOT,
// MKNOD, KILL, AUDIT_*, MAC_*, BLOCK_SUSPEND, WAKE_ALARM, LINUX_IMMUTABLE,
// SYSLOG.
func defaultAllowedCaps() map[string]struct{} {
	return map[string]struct{}{
		"IPC_LOCK":        {}, // mlockall, used by neo4j/postgres
		"SYS_NICE":        {}, // process priority adjustment
		"SYS_RESOURCE":    {}, // raise rlimits beyond defaults
		"DAC_READ_SEARCH": {}, // bypass read permission checks (some DBs)
	}
}

func defaultRules() Rules {
	return Rules{
		AllowedRegistries: map[string]struct{}{
			"docker.io": {}, "ghcr.io": {}, "gcr.io": {},
			"public.ecr.aws": {}, "quay.io": {}, "mcr.microsoft.com": {},
		},
		AllowedEnvVars: map[string]struct{}{},
		AllowedCaps:    defaultAllowedCaps(),
		MaxCPUs:        4,
		MaxMemoryBytes: 8 << 30, // 8 GiB
	}
}

var (
	serviceNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	volNameRe     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	envRefRe      = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)
)

// Parse parses YAML into a Manifest and runs validation.
func Parse(b []byte, rules Rules) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid yaml: %w", err)
	}

	if m.Version != 1 {
		return nil, &ValidationError{
			Code:         "version_unsupported",
			Path:         "version",
			Message:      fmt.Sprintf("unsupported manifest version %d (only version 1 is supported)", m.Version),
			HintForAgent: "set `version: 1` at the top of the manifest",
		}
	}

	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.Struct(&m); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Iterate services in sorted order so error reporting is deterministic
	// for a manifest with multiple problems (otherwise map order leaks into
	// test fixtures and user-visible failure messages).
	names := make([]string, 0, len(m.Services))
	for name := range m.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, sname := range names {
		if !serviceNameRe.MatchString(sname) {
			return nil, &ValidationError{
				Code:         "service_name_invalid",
				Path:         "services." + sname,
				Message:      fmt.Sprintf("service name %q does not match pattern", sname),
				HintForAgent: "service names must start with a lowercase letter and contain only lowercase letters, digits, and hyphens (max 31 chars)",
			}
		}
		svc := m.Services[sname]
		reg, _, _, err := splitImage(svc.Image)
		if err != nil {
			// splitImage failures: distinguish missing tag vs :latest.
			path := fmt.Sprintf("services.%s.image", sname)
			msg := err.Error()
			code := "image_missing_tag"
			hint := "pin the image to an explicit tag, e.g. `redis:7-alpine`"
			if strings.Contains(msg, ":latest") {
				code = "image_latest_forbidden"
				hint = "replace :latest with a specific version tag"
			}
			return nil, &ValidationError{
				Code:         code,
				Path:         path,
				Message:      fmt.Sprintf("services.%s.image: %s", sname, msg),
				HintForAgent: hint,
			}
		}
		if _, ok := rules.AllowedRegistries[reg]; !ok {
			return nil, &ValidationError{
				Code:         "image_registry_not_allowed",
				Path:         fmt.Sprintf("services.%s.image", sname),
				Message:      fmt.Sprintf("services.%s.image: registry %q not in allowlist", sname, reg),
				HintForAgent: "use one of the allowed registries: " + joinKeys(rules.AllowedRegistries),
			}
		}
		for vname, path := range svc.Volumes {
			if !volNameRe.MatchString(vname) {
				return nil, &ValidationError{
					Code:         "volume_invalid_name",
					Path:         fmt.Sprintf("services.%s.volumes.%s", sname, vname),
					Message:      fmt.Sprintf("services.%s.volumes.%s: invalid volume name", sname, vname),
					HintForAgent: "volume names must start with a lowercase letter and contain only lowercase letters, digits, and hyphens (max 31 chars); host paths are not allowed",
				}
			}
			if !strings.HasPrefix(path, "/") {
				return nil, &ValidationError{
					Code:         "volume_path_not_absolute",
					Path:         fmt.Sprintf("services.%s.volumes.%s", sname, vname),
					Message:      fmt.Sprintf("services.%s.volumes.%s: path must be absolute", sname, vname),
					HintForAgent: "container mount paths must be absolute (start with `/`)",
				}
			}
		}
		for ekey, eval := range svc.Env {
			for _, mm := range envRefRe.FindAllStringSubmatch(eval, -1) {
				name := mm[1]
				if _, ok := rules.AllowedEnvVars[name]; !ok {
					return nil, &ValidationError{
						Code:         "env_reference_not_allowed",
						Path:         fmt.Sprintf("services.%s.env.%s", sname, ekey),
						Message:      fmt.Sprintf("services.%s.env.%s: env reference ${%s} is not in the allowlist", sname, ekey, name),
						HintForAgent: "use a literal value or reference an env var in the allowlist",
					}
				}
			}
		}
		for _, c := range svc.CapAdd {
			if _, ok := rules.AllowedCaps[c]; !ok {
				return nil, &ValidationError{
					Code:         "capability_not_allowed",
					Path:         fmt.Sprintf("services.%s.cap_add", sname),
					Message:      fmt.Sprintf("services.%s.cap_add: capability %q is not in the safe-cap allowlist", sname, c),
					HintForAgent: "Allowed caps: IPC_LOCK, SYS_NICE, SYS_RESOURCE, DAC_READ_SEARCH. Other caps require operator approval (file an issue).",
				}
			}
		}
		if svc.Resources != nil {
			if svc.Resources.CPUs != "" {
				n, err := strconv.ParseFloat(svc.Resources.CPUs, 64)
				if err != nil {
					return nil, fmt.Errorf("services.%s.resources.cpus: %w", sname, err)
				}
				if n > rules.MaxCPUs {
					return nil, &ValidationError{
						Code:         "cpu_cap_exceeded",
						Path:         fmt.Sprintf("services.%s.resources.cpus", sname),
						Message:      fmt.Sprintf("services.%s.resources.cpus: %.2f exceeds cap %.2f", sname, n, rules.MaxCPUs),
						HintForAgent: fmt.Sprintf("reduce cpus to at most %.2f", rules.MaxCPUs),
					}
				}
			}
			if bb, err := ParseMemoryBytes(svc.Resources.Memory); err != nil {
				return nil, fmt.Errorf("services.%s.resources.memory: %w", sname, err)
			} else if bb > rules.MaxMemoryBytes {
				return nil, &ValidationError{
					Code:         "memory_cap_exceeded",
					Path:         fmt.Sprintf("services.%s.resources.memory", sname),
					Message:      fmt.Sprintf("services.%s.resources.memory: %d exceeds cap %d", sname, bb, rules.MaxMemoryBytes),
					HintForAgent: fmt.Sprintf("reduce memory to at most %d bytes", rules.MaxMemoryBytes),
				}
			}
		}
	}
	return &m, nil
}

// ParseMemoryBytes parses sizes like "512m" / "2G" / "1024" into bytes.
// Empty input returns 0 without error. Exported so the dockerx package can
// translate validated manifest values into Docker HostConfig.Resources.
func ParseMemoryBytes(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	last := s[len(s)-1]
	num := s
	switch last {
	case 'k', 'K':
		mult = 1 << 10
		num = s[:len(s)-1]
	case 'm', 'M':
		mult = 1 << 20
		num = s[:len(s)-1]
	case 'g', 'G':
		mult = 1 << 30
		num = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("memory %q: %w", s, err)
	}
	return int64(n * float64(mult)), nil
}

// splitImage returns (registry, name, tag). If the registry is omitted
// in the image string, "docker.io" is returned.
func splitImage(img string) (registry, name, tag string, err error) {
	parts := strings.SplitN(img, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", "", fmt.Errorf("image %q missing tag", img)
	}
	if parts[1] == "latest" {
		return "", "", "", fmt.Errorf("image %q uses :latest; pin a version", img)
	}
	tag = parts[1]
	rest := parts[0]

	// First segment is a registry if it has a `.` or `:`; otherwise docker.io.
	segs := strings.SplitN(rest, "/", 2)
	if len(segs) == 2 && strings.ContainsAny(segs[0], ".:") {
		return segs[0], segs[1], tag, nil
	}
	return "docker.io", rest, tag, nil
}
