// Package project owns the host-side per-project lifecycle: enumeration,
// creation, default-project switching, and removal. Each function is the
// authoritative Go implementation of one bash cmd_project_* helper.
package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/okulik/glovebox/internal/dockerx"
)

// Project is a single per-pid project record.
type Project struct {
	PID         string
	Workspace   string
	AgentStatus string // "running", "exited", "absent", ...
	StackStatus string // "no stack" or "<N> containers"
	Active      bool   // true if this pid is the current default
}

// List enumerates state/projects/<pid>/ entries and queries docker for each
// pid's agent + stack status via the supplied dockerx.Client. activePID is the
// current default pid (empty string if no default); the matching Project has
// Active=true.
//
// The slice is sorted by pid (ascending) so output is deterministic.
func List(stateDir, activePID string, dc dockerx.ControllerClient) ([]Project, error) {
	projectsDir := filepath.Join(stateDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}
	ctx := context.Background()
	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		ws, err := readWorkspacePath(projectsDir, pid)
		if err != nil {
			continue
		}
		out = append(out, Project{
			PID:         pid,
			Workspace:   ws,
			AgentStatus: queryAgentStatus(ctx, dc, pid),
			StackStatus: queryStackStatus(ctx, dc, pid),
			Active:      pid == activePID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}

func readWorkspacePath(projectsDir, pid string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectsDir, pid, "workspace-path"))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func queryAgentStatus(ctx context.Context, dc dockerx.ControllerClient, pid string) string {
	if dc == nil {
		return "absent"
	}
	_, state, err := dc.ContainerByName(ctx, "glovebox-agent-"+pid)
	if err != nil || state == "" {
		return "absent"
	}
	return state
}

func queryStackStatus(ctx context.Context, dc dockerx.ControllerClient, pid string) string {
	if dc == nil {
		return "no stack"
	}
	count, exists, err := dc.NetworkContainerCount(ctx, "glovebox-stack-"+pid)
	if err != nil || !exists || count == 1 {
		return "no stack"
	}
	return fmt.Sprintf("%d containers", count)
}
