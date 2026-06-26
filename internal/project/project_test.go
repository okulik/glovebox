package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

func seedProject(t *testing.T, stateDir, pid, workspace string) {
	t.Helper()
	dir := filepath.Join(stateDir, config.ProjectsPath, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.WorkspacePath),
		[]byte(workspace+"\n"), 0o644); err != nil {
		t.Fatalf("write workspace-path: %v", err)
	}
}

// seedAgent registers a fake agent container with the given state. Absent =
// no entry in the Containers map (which is what queryAgentStatus treats as
// "absent" via ContainerByName returning empty state).
func seedAgent(f *dockerx.Fake, pid, state string) {
	cname := config.ContainerAgentPrefix + pid
	f.Containers[cname] = dockerx.FakeContainer{ID: "id-" + cname, State: state}
}

// seedStack registers a per-project stack network with `count` containers
// attached. Absent = no entry, which yields "no stack" via NetworkContainerCount.
func seedStack(f *dockerx.Fake, pid string, count int) {
	f.NetworkContainers[config.ContainerStackPrefix+pid] = count
}

func TestListEmptyWhenProjectsDirMissing(t *testing.T) {
	state := t.TempDir()
	projects, err := List(state, "", dockerx.NewFake())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("want 0 projects, got %d", len(projects))
	}
}

func TestListReturnsProjects(t *testing.T) {
	state := t.TempDir()
	seedProject(t, state, "aaaa1111bbbb", "/work/foo")
	seedProject(t, state, "cccc2222dddd", "/work/bar")
	f := dockerx.NewFake()
	seedAgent(f, "aaaa1111bbbb", string(container.StateRunning))
	seedAgent(f, "cccc2222dddd", string(container.StateExited))
	seedStack(f, "aaaa1111bbbb", 2)
	// cccc2222dddd intentionally absent → "no stack"

	projects, err := List(state, "", f)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("want 2 projects, got %d: %+v", len(projects), projects)
	}
	if projects[0].PID != "aaaa1111bbbb" || projects[1].PID != "cccc2222dddd" {
		t.Errorf("unexpected order: %+v", projects)
	}
	if projects[0].AgentStatus != string(container.StateRunning) {
		t.Errorf("agent status: want running, got %q", projects[0].AgentStatus)
	}
	if projects[0].StackStatus != "2 containers" {
		t.Errorf("stack status: want '2 containers', got %q", projects[0].StackStatus)
	}
	if projects[1].StackStatus != "no stack" {
		t.Errorf("inactive stack: want 'no stack', got %q", projects[1].StackStatus)
	}
}

func TestListMarksActive(t *testing.T) {
	state := t.TempDir()
	seedProject(t, state, "aaaa1111bbbb", "/work/foo")
	f := dockerx.NewFake()
	seedAgent(f, "aaaa1111bbbb", string(container.StateRunning))
	projects, err := List(state, "aaaa1111bbbb", f)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !projects[0].Active {
		t.Error("want Active=true")
	}
}
