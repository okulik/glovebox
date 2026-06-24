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
//
// Intended usage: route every taint-derived path through UnderBase before
// passing it to os.{Read,Write,Open,Create}File, so a misconfigured
// GBX_* env var or hostile CWD produces a clean refusal instead of a
// directory-escaping file operation.
//
// Both arguments are filepath.Clean'd first, so callers don't have to
// worry about trailing slashes or "/." spelling differences.
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
	// Append sep to both sides so "/foo/bar" is not accepted as inside
	// "/foo/ba" (prefix match without a boundary token would).
	if !strings.HasPrefix(absCandidate+sep, absBase+sep) {
		return "", fmt.Errorf("path %q escapes base %q", absCandidate, absBase)
	}
	return absCandidate, nil
}

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

// deepMergeJSON merges the default value src into the existing value dst and
// returns the result, following glovebox's reconcile rules:
//
//   - objects (map[string]any): merged recursively; keys only in src are added,
//     keys in dst are kept (recursively merged when both are objects).
//   - arrays ([]any): unioned - every dst element is kept and each src element
//     not already present (by deep equality) is appended. This makes list-valued
//     config such as permissions.allow additive.
//   - anything else (scalars, or a dst/src type mismatch): dst wins.
//
// The user's existing value is never overwritten; only missing keys and missing
// array elements are filled in from the default.
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
