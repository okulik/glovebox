package project

import (
	//nolint:gosec // G505: SHA-1 used as identifier hash, not crypto.
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Hash(path string) (string, error) {
	abs, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	//nolint:gosec
	sum := sha1.Sum([]byte(abs))
	return hex.EncodeToString(sum[:])[:12], nil
}

// Resolve scans stateDir/projects/ and returns the unique pid whose name has
// prefix.
func Resolve(stateDir, prefix string) (string, error) {
	projects := filepath.Join(stateDir, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("No project matches: %s", prefix)
		}
		return "", fmt.Errorf("read projects dir: %w", err)
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("No project matches: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("Project id is ambiguous: %s (matches: %s)", prefix, strings.Join(matches, " "))
	}
}
