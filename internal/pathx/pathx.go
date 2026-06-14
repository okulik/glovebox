// Package pathx provides path-safety helpers used to satisfy gosec's
// path-traversal-via-taint check (G703) at sites where a destination
// path is derived from operator-influenced input (CWD, GBX_* env vars).
//
// The package exists so the same containment pattern doesn't get
// reimplemented at every taint sink - a single helper keeps the policy
// (and the prefix-with-separator subtlety that prevents
// "/foo/bar-evil" matching "/foo/bar") in exactly one place.
package pathx

import (
	"fmt"
	"os"
	"path/filepath"
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
