package convexport

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/okulik/glovebox/internal/agent"
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

// harness knows how to relocate one agent's in-sandbox logs onto the host.
type harness interface {
	name() string
	supported() bool
	// destRoot is the native host root for this harness (e.g. ~/.claude).
	destRoot(home string) string
	// export copies logs from srcDir (=<proj>/<harness>) into root, rewriting
	// them so the viewer attributes each session to a glovebox-tagged project.
	export(srcDir, root, pid, workspace string) (int, error)
}

// registry builds one exporter per known agent, derived from the canonical set
// so it can't drift.
func registry() []harness {
	out := make([]harness, 0, len(agent.Names))
	for _, name := range agent.Names {
		if name == "claude" {
			out = append(out, claude{})
			continue
		}
		out = append(out, unsupported(name))
	}
	return out
}

// ExportProject exports one project's conversation logs.
func ExportProject(stateDir, home, pid, only, destOverride string) ([]Result, error) {
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
		n, err := h.export(srcDir, root, pid, workspace)
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

// Slugify encodes a path the way Claude Code names its project folders: every
// non-alphanumeric character becomes a dash. So "/workspace" -> "-workspace".
func Slugify(p string) string { return nonAlnum.ReplaceAllString(p, "-") }

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// provenanceRoot is the host path that a project's in-sandbox cwd (/workspace)
// is rewritten to. Viewers like AgentsView derive the project name from the
// basename of a session's cwd, so we tag that basename with "gbx-<pid>-":
//
//	/Users/you/code/gwook  ->  /Users/you/code/gbx-<pid>-gwook
func provenanceRoot(pid, workspace string) string {
	tag := "gbx-" + pid
	if workspace == "" {
		return "/" + tag
	}
	return filepath.Join(filepath.Dir(workspace), tag+"-"+filepath.Base(workspace))
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

func (claude) export(srcDir, root, pid, workspace string) (int, error) {
	projectsSrc := filepath.Join(srcDir, "projects")
	entries, err := os.ReadDir(projectsSrc)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	newRoot := provenanceRoot(pid, workspace)
	baseSlug := Slugify(newRoot)
	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// The in-sandbox cwd-slug is "-workspace" (or "-workspace-<sub>" when the
		// agent ran from a subdir).
		suffix := strings.TrimPrefix(e.Name(), "-workspace")
		destDir := filepath.Join(root, "projects", baseSlug+suffix)
		n, err := copyRewriteJSONL(filepath.Join(projectsSrc, e.Name()), destDir, newRoot)
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
func (unsupported) export(string, string, string, string) (int, error) {
	return 0, nil
}

// copyRewriteJSONL copies every *.jsonl file from srcDir into destDir (creating
// it, overwriting existing files), rewriting the in-sandbox cwd to newRoot, and
// returns the count copied.
func copyRewriteJSONL(srcDir, destDir, newRoot string) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return n, err
		}
		if err := os.WriteFile(filepath.Join(destDir, e.Name()), rewriteCwd(data, newRoot), 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// rewriteCwd retargets the sandbox cwd in a Claude session log. Records carry
// a `"cwd":"/workspace"` field (or "/workspace/<sub>"); we repoint just that
// field to newRoot so the viewer attributes the session correctly.
func rewriteCwd(data []byte, newRoot string) []byte {
	old := []byte(`"cwd":"/workspace`)
	neu := []byte(`"cwd":"` + newRoot)
	return bytes.ReplaceAll(data, old, neu)
}
