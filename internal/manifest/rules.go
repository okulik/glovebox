package manifest

import (
	"bufio"
	"errors"
	"os"
	"sort"
	"strings"
)

// LoadRules reads an allowlist file (one registry per line; `#` comments) and
// returns Rules combining the loaded registries with the supplied caps.
func LoadRules(path string, maxCPUs float64, maxMemBytes int64) (Rules, error) {
	f, err := os.Open(path)
	if err != nil {
		return Rules{}, err
	}
	defer f.Close()

	r := Rules{
		AllowedRegistries: map[string]struct{}{},
		AllowedEnvVars:    map[string]struct{}{},
		AllowedCaps:       defaultAllowedCaps(),
		MaxCPUs:           maxCPUs,
		MaxMemoryBytes:    maxMemBytes,
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r.AllowedRegistries[line] = struct{}{}
	}
	return r, sc.Err()
}

// ValidationError is the structured error type the API surfaces to clients.
type ValidationError struct {
	Code         string
	Path         string
	Message      string
	HintForAgent string
}

func (e *ValidationError) Error() string { return e.Message }

// AsValidationError unwraps an error chain and returns the first
// *ValidationError, or nil if there isn't one.
func AsValidationError(err error) *ValidationError {
	var v *ValidationError
	if errors.As(err, &v) {
		return v
	}
	return nil
}

func joinKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
