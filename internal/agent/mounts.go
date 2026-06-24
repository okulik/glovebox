package agent

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mount is one host:container bind mount applied to the agent container in
// addition to the fixed set built into BuildCreateArgs. Host is the resolved
// absolute path on the host; Container is an absolute path inside the
// container; Mode is either "rw" or "ro".
type Mount struct {
	Host      string
	Container string
	Mode      string
}

// String returns the canonical "host:container:mode" form used both as the
// docker --volume argument value and as the on-disk serialization.
func (m Mount) String() string {
	return fmt.Sprintf("%s:%s:%s", m.Host, m.Container, m.Mode)
}

// reservedContainerPaths are absolute container paths claimed by the fixed
// mounts in BuildCreateArgs. ParseMountSpec / Validate refuse to shadow them.
var reservedContainerPaths = map[string]bool{
	"/workspace":                      true,
	"/workspace/.claude":              true,
	"/etc/glovebox/docker-sandbox.md": true,
	"/etc/glovebox/proxy-sandbox.md":  true,
	"/home/gbx/.claude":               true,
	"/home/gbx/.claude.json":          true,
	"/home/gbx/.codex":                true,
	"/home/gbx/.aider":                true,
	"/home/gbx/.local/share/opencode": true,
	"/home/gbx/.pi":                   true,
	"/home/gbx/.gemini":               true,
	"/home/gbx/.hermes":               true,
	"/home/gbx/.npm":                  true,
	"/home/gbx/.local/share/uv-tools": true,
	"/home/gbx/.local/bin":            true,
	"/home/gbx/.cache":                true,
	"/home/gbx/.shell-history":        true,
}

// ParseMountSpec accepts one of the three user-facing forms:
//
//	host
//	host:container
//	host:container:mode
//
// The host is resolved via filepath.EvalSymlinks so the recorded source
// matches what docker actually mounts. When the container is omitted it
// defaults to /mnt/<basename(host)>. Mode defaults to "rw"; only "rw" and
// "ro" are accepted.
func ParseMountSpec(s string) (Mount, error) {
	if s == "" {
		return Mount{}, errors.New("empty mount spec")
	}
	parts := strings.Split(s, ":")
	var host, container, mode string
	switch len(parts) {
	case 1:
		host = parts[0]
	case 2:
		host, container = parts[0], parts[1]
	case 3:
		host, container, mode = parts[0], parts[1], parts[2]
	default:
		return Mount{}, fmt.Errorf("invalid mount spec %q: want host[:container][:rw|ro]", s)
	}
	if host == "" {
		return Mount{}, fmt.Errorf("invalid mount spec %q: empty host path", s)
	}
	resolved, err := filepath.EvalSymlinks(host)
	if err != nil {
		return Mount{}, fmt.Errorf("resolve host path %q: %w", host, err)
	}
	if container == "" {
		container = "/mnt/" + filepath.Base(resolved)
	}
	if !strings.HasPrefix(container, "/") {
		return Mount{}, fmt.Errorf("container path %q must be absolute", container)
	}
	container = filepath.Clean(container)
	if mode == "" {
		mode = "rw"
	}
	if mode != "rw" && mode != "ro" {
		return Mount{}, fmt.Errorf("mount mode %q: want rw or ro", mode)
	}
	if reservedContainerPaths[container] {
		return Mount{}, fmt.Errorf("container path %q is reserved by the agent runtime", container)
	}
	return Mount{Host: resolved, Container: container, Mode: mode}, nil
}

// mountsFilename is the on-disk file name under ${stateProjDir}/.
const mountsFilename = "mounts.txt"

// ReadMounts loads the project's mount set from ${stateProjDir}/mounts.txt.
// A missing file is not an error: the project has no extra mounts. Blank
// lines and "#"-prefixed comments are ignored.
func ReadMounts(stateProjDir string) ([]Mount, error) {
	path := filepath.Join(stateProjDir, mountsFilename)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	var out []Mount
	sc := bufio.NewScanner(f)
	lineno := 0
	for sc.Scan() {
		lineno++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d: invalid entry %q (want host:container:mode)", path, lineno, line)
		}
		m := Mount{Host: parts[0], Container: parts[1], Mode: parts[2]}
		if m.Host == "" || !strings.HasPrefix(m.Container, "/") {
			return nil, fmt.Errorf("%s:%d: invalid entry %q", path, lineno, line)
		}
		if m.Mode != "rw" && m.Mode != "ro" {
			return nil, fmt.Errorf("%s:%d: invalid mode %q", path, lineno, m.Mode)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// WriteMounts replaces ${stateProjDir}/mounts.txt with one canonical line per
// entry. The write is atomic: a sibling tempfile is renamed over the
// destination. The destination directory must already exist.
func WriteMounts(stateProjDir string, mounts []Mount) error {
	path := filepath.Join(stateProjDir, mountsFilename)
	var buf strings.Builder
	for _, m := range mounts {
		buf.WriteString(m.String())
		buf.WriteString("\n")
	}
	return WriteAtomic(path, []byte(buf.String()), 0o600)
}
