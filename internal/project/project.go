// Package project owns the host-side per-project lifecycle:.
package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

const (
	listTimeout = time.Second * 30
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
// pid's agent + stack status.
func List(stateDir, activePID string, dc dockerx.ControllerClient) ([]Project, error) {
	projectsDir := filepath.Join(stateDir, config.ProjectsPath)
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()

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
	data, err := os.ReadFile(filepath.Join(projectsDir, pid, config.WorkspacePath))
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(data), "\n"), nil
}

func queryAgentStatus(ctx context.Context, dc dockerx.ControllerClient, pid string) string {
	if dc == nil {
		return "absent"
	}

	_, state, err := dc.ContainerByName(ctx, config.ContainerAgentPrefix+pid)
	if err != nil || state == "" {
		return "absent"
	}

	return state
}

func queryStackStatus(ctx context.Context, dc dockerx.ControllerClient, pid string) string {
	if dc == nil {
		return "no stack"
	}

	count, exists, err := dc.NetworkContainerCount(ctx, config.ContainerStackPrefix+pid)
	if err != nil || !exists || count == 1 {
		return "no stack"
	}

	return fmt.Sprintf("%d containers", count)
}
