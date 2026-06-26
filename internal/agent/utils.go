package agent

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// UnderBase returns the cleaned absolute form of candidate iff it
// resolves to a location strictly inside base (or equals base itself);
// otherwise it returns an error.
func UnderBase(base, candidate string) (string, error) {
	absBase, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return "", fmt.Errorf("resolve base %q: %w", base, err)
	}
	absCandidate, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve candidate %q: %w", candidate, err)
	}
	if absCandidate == absBase {
		return absCandidate, nil
	}
	sep := string(os.PathSeparator)
	if !strings.HasPrefix(absCandidate+sep, absBase+sep) {
		return "", fmt.Errorf("path %q escapes base %q", absCandidate, absBase)
	}
	return absCandidate, nil
}

// WriteAtomic writes data to path durably-enough for glovebox's needs: it
// creates a sibling temp file in filepath.Dir(path), writes data, sets the
// file's mode to perm exactly (independent of umask), then renames the temp
// file over path.
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

// deepMergeJSON merges the default value src into the existing value dst and
// returns the result.
func DeepMergeJSON(dst, src any) any {
	switch d := dst.(type) {
	case map[string]any:
		s, ok := src.(map[string]any)
		if !ok {
			return d
		}
		out := make(map[string]any, len(d))
		maps.Copy(out, d)
		for k, sv := range s {
			if dv, exists := out[k]; exists {
				out[k] = DeepMergeJSON(dv, sv)
			} else {
				out[k] = sv
			}
		}
		return out
	case []any:
		s, ok := src.([]any)
		if !ok {
			return d
		}
		out := append([]any(nil), d...)
		for _, sv := range s {
			found := false
			for _, dv := range out {
				if reflect.DeepEqual(dv, sv) {
					found = true
					break
				}
			}
			if !found {
				out = append(out, sv)
			}
		}
		return out
	default:
		return d
	}
}
