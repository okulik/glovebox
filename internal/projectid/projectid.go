// Package projectid owns the rules for deriving a glovebox project id from a
// workspace path. A pid is the first 12 lowercase hex characters of the SHA-1
// of the path's resolved absolute form.
package projectid

import (
	// SHA-1 is used here as a stable IDENTIFIER hash, not for any
	// security/integrity property. Pids are 12 hex chars used as names for
	// state directories and Docker containers; collision risk on absolute
	// path strings is negligible at this size and there's no adversarial
	// model. Switching algorithms would invalidate every existing pid on
	// every install, so this stays.
	//nolint:gosec // G505: SHA-1 used as identifier hash, not crypto.
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hash returns the 12-character lowercase hex pid for path. The path is
// resolved with filepath.EvalSymlinks; the resolved form is the input to SHA-1.
func Hash(path string) (string, error) {
	abs, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	//nolint:gosec // G401: identifier hash, not cryptographic; see import comment.
	sum := sha1.Sum([]byte(abs))
	return hex.EncodeToString(sum[:])[:12], nil
}

// Resolve scans stateDir/projects/ and returns the unique pid whose name has
// prefix. The error messages are part of gbx's user-facing contract; the
// 90 / 91 / AAH tests pin specific substrings:
//   - no matches: "No project matches: <prefix>"
//   - ambiguous: "Project id is ambiguous: <prefix> (matches: <a b c>)"
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
