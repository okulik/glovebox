// Package hostconfig owns the idempotent bootstrap of the user's
// ${GBX_CONFIG_DIR} directory: state skeleton, .env, and allowlist files.
// It does NOT touch project containers - that lives in internal/agent.
package hostconfig

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Bootstrap performs the idempotent bootstrap of configDir:
//   - state/ skeleton (projects/, shared/{npm,uv-tools,bin,cache,shell-history})
//   - .env (copied from <libexec>/.env.example if missing)
//   - allowlist.txt (copied from <libexec>/docker/proxy/allowlist.txt if missing)
//   - SyncEnvFile: append missing keys from .env.example.
//
// All operations are safe to repeat. User edits to seeded files are preserved.
func Bootstrap(libexec, configDir string) error {
	if err := mkStateSkeleton(configDir); err != nil {
		return err
	}
	if err := seedIfMissing(filepath.Join(libexec, ".env.example"),
		filepath.Join(configDir, ".env")); err != nil {
		return fmt.Errorf("seed .env: %w", err)
	}
	if err := seedIfMissing(filepath.Join(libexec, "docker", "proxy", "allowlist.txt"),
		filepath.Join(configDir, "allowlist.txt")); err != nil {
		return fmt.Errorf("seed allowlist.txt: %w", err)
	}
	if err := syncEnvFile(filepath.Join(libexec, ".env.example"),
		filepath.Join(configDir, ".env")); err != nil {
		return fmt.Errorf("sync .env: %w", err)
	}
	return nil
}

func mkStateSkeleton(configDir string) error {
	dirs := []string{
		"state/projects",
		"state/shared/npm",
		"state/shared/uv-tools",
		"state/shared/bin",
		"state/shared/cache",
		"state/shared/shell-history",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(configDir, d), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func seedIfMissing(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// syncEnvFile appends to dst any KEY= lines from src whose KEY is not yet
// present in dst. Comments and blank lines in src are ignored.
func syncEnvFile(srcExample, dstEnv string) error {
	srcKeys, srcLines, err := readEnvKeys(srcExample)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	_, dstKeys, err := readEnvKeys(dstEnv)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var toAppend []string
	for _, key := range srcKeys {
		if _, has := dstKeys[key]; !has {
			toAppend = append(toAppend, srcLines[key])
		}
	}
	if len(toAppend) == 0 {
		return nil
	}
	f, err := os.OpenFile(dstEnv, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("\n# Added by bootstrap from the current template\n"); err != nil {
		return err
	}
	for _, line := range toAppend {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// readEnvKeys returns (ordered keys, key->originalLine). Keys appear in the
// order they were read so syncEnvFile preserves source ordering.
func readEnvKeys(path string) ([]string, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var keys []string
	lines := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := line[:idx]
		if _, has := lines[key]; !has {
			keys = append(keys, key)
			lines[key] = line
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return keys, lines, nil
}
