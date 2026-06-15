// Package fsx holds small filesystem helpers shared across glovebox's internal
// packages. It is a leaf package: it imports only the standard library so that
// other internal packages may depend on it without introducing import cycles.
package fsx

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to path durably-enough for glovebox's needs: it
// creates a sibling temp file in filepath.Dir(path), writes data, sets the
// file's mode to perm exactly (independent of umask), then renames the temp
// file over path. Concurrent readers therefore observe either the old or the
// new content, never a torn write.
//
// The temp file is removed on every error path (write, chmod, close, rename).
// There is no fsync: this matches the prior hand-rolled call sites and trades a
// crash-durability guarantee for speed, which is acceptable for config-style
// files that are regenerated on demand.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod %s: %w", tmpName, err)
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
