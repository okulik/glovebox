package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/config"
)

const (
	dockerfileName = "Dockerfile.plugins"
	noDescription  = "(no description)"
)

// Dir returns the plugins directory for a project's state dir.
func Dir(stateProjDir string) string {
	return filepath.Join(stateProjDir, config.PluginsPath)
}

// List returns the project's plugins sorted by id. A missing plugins dir is
// not an error: it yields an empty slice. Hidden files (e.g. editor drafts
// named ".draft-*") are ignored.
func List(stateProjDir string) ([]Plugin, error) {
	dir := Dir(stateProjDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}
	var out []Plugin
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read plugin %s: %w", e.Name(), err)
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat plugin %s: %w", e.Name(), err)
		}
		// A stored plugin always has a valid description, but tolerate a
		// hand-edited file by falling back to a placeholder rather than failing
		// the whole listing.
		desc, derr := ParseDescription(string(data))
		if derr != nil {
			desc = noDescription
		}
		out = append(out, Plugin{
			ID:          e.Name(),
			Description: desc,
			Path:        path,
			Content:     string(data),
			ModTime:     info.ModTime(),
		})
	}
	slices.SortFunc(out, func(a, b Plugin) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Find resolves a plugin id prefix to a single plugin within the project.
func Find(stateProjDir, prefix string) (Plugin, error) {
	list, err := List(stateProjDir)
	if err != nil {
		return Plugin{}, err
	}
	var matches []Plugin
	for _, p := range list {
		if strings.HasPrefix(p.ID, prefix) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return Plugin{}, fmt.Errorf("No plugin matches: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return Plugin{}, fmt.Errorf("Plugin id is ambiguous: %s (matches: %s)", prefix, strings.Join(ids, " "))
	}
}

// Store validates content and writes it as a new plugin under the project's
// plugins dir, returning the new id.
func Store(stateProjDir, content string, ts time.Time) (string, error) {
	if err := Validate(content); err != nil {
		return "", err
	}
	dir := Dir(stateProjDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create plugins dir: %w", err)
	}
	id := HashID(content, ts)
	if err := agent.WriteAtomic(filepath.Join(dir, id), []byte(content), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

// Overwrite validates content and rewrites an existing plugin in place,
// preserving its id.
func Overwrite(p Plugin, content string) error {
	if err := Validate(content); err != nil {
		return err
	}
	return agent.WriteAtomic(p.Path, []byte(content), 0o600)
}

// Remove deletes a plugin's fragment file.
func Remove(p Plugin) error {
	if err := os.Remove(p.Path); err != nil {
		return fmt.Errorf("remove plugin %s: %w", p.ID, err)
	}
	return nil
}

// WriteDockerfile renders and writes the layered Dockerfile for plugins into
// <stateProjDir>/Dockerfile.plugins and returns its path.
func WriteDockerfile(stateProjDir, base string, plugins []Plugin) (string, error) {
	path := filepath.Join(stateProjDir, dockerfileName)
	if err := agent.WriteAtomic(path, []byte(GenerateDockerfile(base, plugins)), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
