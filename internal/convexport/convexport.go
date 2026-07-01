package convexport

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/okulik/glovebox/internal/config"
)

const (
	StatusExported    = "exported"    // logs were copied
	StatusEmpty       = "empty"       // harness dir present but no conversations
	StatusUnsupported = "unsupported" // harness exporter not yet implemented
)

// Result summarizes one harness's export for one project.
type Result struct {
	Harness string
	PID     string
	Files   int
	Status  string
	DestDir string // root the files were copied under (StatusExported only)
	Note    string
}

var HarnessNames = []string{"claude", "codex", "gemini", "opencode", "aider", "pi", "hermes"}

// harness knows how to relocate one agent's in-sandbox logs onto the host.
type harness interface {
	name() string
	supported() bool
	// destRoot is the native host root for this harness (e.g. ~/.claude).
	destRoot(home string) string
	// export transfers logs from srcDir (=<proj>/<harness>) into root, tagging
	// them with the project's pid and real host workspace.
	export(srcDir, root, pid, workspace string, doCopy bool) (int, error)
}

func registry() []harness {
	return []harness{
		claude{},
		// Scaffolded - each needs its own host root + provenance scheme, and
		// real glovebox sample data to verify against before enabling.
		unsupported("codex"), unsupported("gemini"), unsupported("opencode"),
		unsupported("aider"), unsupported("pi"), unsupported("hermes"),
	}
}

// ExportProject exports one project's conversation logs.
func ExportProject(stateDir, home, pid, only, destOverride string, doCopy bool) ([]Result, error) {
	projDir := filepath.Join(stateDir, config.ProjectsPath, pid)
	workspace := readWorkspace(projDir)

	var results []Result
	for _, h := range registry() {
		if only != "" && h.name() != only {
			continue
		}
		res := Result{Harness: h.name(), PID: pid}

		if !h.supported() {
			res.Status = StatusUnsupported
			res.Note = "exporter not yet implemented"
			results = append(results, res)
			continue
		}

		srcDir := filepath.Join(projDir, h.name())
		if _, err := os.Stat(srcDir); errors.Is(err, fs.ErrNotExist) {
			res.Status = StatusEmpty
			results = append(results, res)
			continue
		}

		root := destOverride
		if root == "" {
			root = h.destRoot(home)
		}
		n, err := h.export(srcDir, root, pid, workspace, doCopy)
		if err != nil {
			return results, fmt.Errorf("export %s for project %s: %w", h.name(), pid, err)
		}
		if n == 0 {
			res.Status = StatusEmpty
		} else {
			res.Status = StatusExported
			res.Files = n
			res.DestDir = root
		}
		results = append(results, res)
	}
	return results, nil
}

// RemoveExports deletes the symlinks a prior export created for one project,
// across every harness's host root, and prunes any folders left empty.
func RemoveExports(home, pid string) (int, error) {
	base := Slugify(syntheticBase(pid, "")) // "-glovebox-<pid>"
	removed := 0
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, h := range registry() {
		root := h.destRoot(home)
		if root == "" {
			continue
		}
		projectsDir := filepath.Join(root, "projects")
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			continue // no exports for this harness
		}
		for _, e := range entries {
			// A project's sessions live under "-glovebox-<pid>" exactly, or
			// "-glovebox-<pid>-<...>" for a real workspace / subdir.
			if e.Name() != base && !strings.HasPrefix(e.Name(), base+"-") {
				continue
			}
			dir := filepath.Join(projectsDir, e.Name())
			n, err := removeSymlinksAndPrune(dir)
			note(err)
			removed += n
		}
	}
	return removed, firstErr
}

// removeSymlinksAndPrune deletes symlink entries directly under dir and removes
// dir itself if that leaves it empty (a copy left behind keeps the dir).
func removeSymlinksAndPrune(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	var firstErr error
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	// Prune the dir only if nothing (copies, other files) remains.
	if rest, err := os.ReadDir(dir); err == nil && len(rest) == 0 {
		_ = os.Remove(dir)
	}
	return removed, firstErr
}

// Slugify encodes a path the way Claude Code names its project folders: every
// non-alphanumeric character becomes a dash. So "/workspace" -> "-workspace".
func Slugify(p string) string { return nonAlnum.ReplaceAllString(p, "-") }

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// syntheticBase is the absolute path whose slug tags a project's exported
// sessions: "/glovebox/<pid>" followed by the real workspace path.
func syntheticBase(pid, workspace string) string {
	return "/glovebox/" + pid + workspace
}

// readWorkspace returns the project's real host workspace path, or "" if the
// marker file is missing/empty (export still proceeds with a pid-only slug).
func readWorkspace(projDir string) string {
	data, err := os.ReadFile(filepath.Join(projDir, config.WorkspacePath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// claude exports Claude Code sessions. Layout: <src>/projects/<cwd-slug>/<uuid>.jsonl,
// where the in-sandbox cwd-slug is "-workspace" (or "-workspace-<sub>" when the
// agent ran from a subdir).
type claude struct{}

func (claude) name() string                { return "claude" }
func (claude) supported() bool             { return true }
func (claude) destRoot(home string) string { return filepath.Join(home, ".claude") }

func (claude) export(srcDir, root, pid, workspace string, doCopy bool) (int, error) {
	projectsSrc := filepath.Join(srcDir, "projects")
	entries, err := os.ReadDir(projectsSrc)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	baseSlug := Slugify(syntheticBase(pid, workspace))
	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// "-workspace" -> "", "-workspace-frontend" -> "-frontend".
		suffix := strings.TrimPrefix(e.Name(), "-workspace")
		destDir := filepath.Join(root, "projects", baseSlug+suffix)
		n, err := transferJSONL(filepath.Join(projectsSrc, e.Name()), destDir, doCopy)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// unsupported is a scaffold for harnesses not yet implemented.
type unsupported string

func (u unsupported) name() string         { return string(u) }
func (unsupported) supported() bool        { return false }
func (unsupported) destRoot(string) string { return "" }
func (unsupported) export(string, string, string, string, bool) (int, error) {
	return 0, nil
}

// transferJSONL relocates every *.jsonl file from srcDir into destDir,
// symlinking (doCopy=false) or copying, and returns the count transferred.
func transferJSONL(srcDir, destDir string, doCopy bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(destDir, e.Name())
		if err := transferFile(src, dst, doCopy); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func transferFile(src, dst string, doCopy bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Replace any prior export so copy<->symlink switches and re-runs are clean.
	if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if !doCopy {
		// Absolute target so the link resolves regardless of the viewer's cwd.
		abs, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		return os.Symlink(abs, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
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
	return out.Close()
}
