package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// dockerfileName is the generated layered Dockerfile written into the project
// state dir (the parent of the plugins dir).
const (
	dockerfileName = "Dockerfile.plugins"
	noDescription  = "(no description)"
)

// Dir returns the plugins directory for a project's state dir.
func Dir(stateProjDir string) string {
	return filepath.Join(stateProjDir, "plugins")
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
// The "No plugin matches" / "Plugin id is ambiguous" strings are deliberately
// capitalized and are part of gbx's user-facing output (mirroring
// projectid.Resolve); keep them as-is rather than "fixing" the capitalization.
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
// plugins dir, returning the new id. The write is atomic (temp + rename).
func Store(stateProjDir, content string, ts time.Time) (string, error) {
	if err := Validate(content); err != nil {
		return "", err
	}
	dir := Dir(stateProjDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create plugins dir: %w", err)
	}
	id := HashID(content, ts)
	if err := atomicWrite(filepath.Join(dir, id), content); err != nil {
		return "", err
	}
	return id, nil
}

// Overwrite validates content and rewrites an existing plugin in place,
// preserving its id. The write is atomic.
func Overwrite(p Plugin, content string) error {
	if err := Validate(content); err != nil {
		return err
	}
	return atomicWrite(p.Path, content)
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
	if err := atomicWrite(path, GenerateDockerfile(base, plugins)); err != nil {
		return "", err
	}
	return path, nil
}

// atomicWrite writes content to path via a sibling temp file + rename.
func atomicWrite(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".write-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}
