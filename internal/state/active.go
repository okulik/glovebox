// Package state owns the on-disk layout of the host-side glovebox config dir,
// including the default-project pointer at ${CONFIG_DIR}/active-project.
//
// The active-project file is a two-line text file:
//
//	line 1: pid
//	line 2: absolute workspace path
//
// A missing file means "no default project is set"; both readers return an
// empty string with no error in that case.
package state

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ActivePID returns the pid recorded in ${configDir}/active-project, or the
// empty string if the file does not exist.
func ActivePID(configDir string) (string, error) {
	return readActiveLine(configDir, 0)
}

// ActivePath returns the workspace path recorded in ${configDir}/active-project,
// or the empty string if the file does not exist or has only one line.
func ActivePath(configDir string) (string, error) {
	return readActiveLine(configDir, 1)
}

func readActiveLine(configDir string, idx int) (string, error) {
	f, err := os.Open(filepath.Join(configDir, "active-project"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for i := 0; i <= idx; i++ {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", nil
		}
	}
	return scanner.Text(), nil
}

// WriteActive atomically replaces ${configDir}/active-project with a two-line
// file containing pid and workspace. The config directory is created if it
// does not already exist (0o755). The write is done by writing to a sibling
// tmpfile then renaming it, so concurrent readers always see either the old
// or new content, never a torn write.
func WriteActive(configDir, pid, workspace string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	final := filepath.Join(configDir, "active-project")
	tmp := fmt.Sprintf("%s.%d.tmp", final, os.Getpid())
	content := []byte(pid + "\n" + workspace + "\n")
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp -> active-project: %w", err)
	}
	return nil
}

// removedDefaultMarker is a one-shot file written by project.Remove when it
// clears the active-project pointer because the removed pid was the default.
// project.New reads and unlinks it: if the new project's pid matches, the
// caller's earlier "demote" intent is honored - the new project is registered
// without being auto-set as default. Bumping the file format requires adjusting
// the read/write side together; the format is a single 12-char hex pid.
const removedDefaultMarker = "last-removed-default-pid"

// MarkRemovedDefault writes the pid of the project the user just removed-while-
// it-was-default. Overwrites any prior marker (one-shot, one-pid). Safe to
// call on a missing config dir - it is created if needed.
func MarkRemovedDefault(configDir, pid string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	final := filepath.Join(configDir, removedDefaultMarker)
	tmp := fmt.Sprintf("%s.%d.tmp", final, os.Getpid())
	if err := os.WriteFile(tmp, []byte(pid+"\n"), 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp -> %s: %w", removedDefaultMarker, err)
	}
	return nil
}

// ConsumeRemovedDefault reads the marker and removes it. Returns an empty pid
// (with nil error) when no marker is present.
func ConsumeRemovedDefault(configDir string) (string, error) {
	path := filepath.Join(configDir, removedDefaultMarker)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	_ = os.Remove(path)
	pid := string(data)
	for len(pid) > 0 && (pid[len(pid)-1] == '\n' || pid[len(pid)-1] == '\r') {
		pid = pid[:len(pid)-1]
	}
	return pid, nil
}

// ClearRemovedDefault drops the marker without returning it. Used by
// `project use` since an explicit user-set default overrides any prior
// remove-as-default intent.
func ClearRemovedDefault(configDir string) error {
	err := os.Remove(filepath.Join(configDir, removedDefaultMarker))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
