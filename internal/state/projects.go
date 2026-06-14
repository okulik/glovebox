package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RequireSomeProject returns the canonical "no projects" error if
// ${stateDir}/projects/ does not exist or contains no entries. The error
// message text is part of gbx's user-facing contract (test AAF asserts the
// substring "No projects. Run 'gbx new <path>' first.").
func RequireSomeProject(stateDir string) error {
	const msg = "No projects. Run 'gbx new <path>' first."
	projects := filepath.Join(stateDir, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New(msg)
		}
		return fmt.Errorf("read projects dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil
		}
	}
	return errors.New(msg)
}
