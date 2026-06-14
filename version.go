// Package glovebox is the module-root package. It exists to hold cross-binary
// constants - chiefly the project version embedded from version.txt - that
// would otherwise need to be duplicated under cmd/gbx and cmd/gbxa.
package glovebox

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var rawVersion string

// Version returns the semver string written in version.txt with surrounding
// whitespace trimmed. `make release` bumps that file and tags the commit.
func Version() string {
	return strings.TrimSpace(rawVersion)
}
