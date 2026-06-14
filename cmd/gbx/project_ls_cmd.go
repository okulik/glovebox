package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/project"
	"github.com/okulik/glovebox/internal/state"
)

type ProjectLsCmd struct {
	Verbose bool `short:"v" name:"verbose" help:"Tree view: each project with its agent + stack containers (name, image, status, tag)."`
	JSON    bool `name:"json" help:"Emit the full listing (projects + containers + others) as a JSON document. Suppresses color and overrides --verbose."`
}

func (c *ProjectLsCmd) Run(kctx *kong.Context) error {
	cfg := configDirFromEnv()
	activePID, err := state.ActivePID(cfg)
	if err != nil {
		return fmt.Errorf("can't read active pid: %w", err)
	}
	dc := projectClient()
	projects, err := project.List(stateDirFromEnv(), activePID, dc)
	if err != nil {
		return err
	}
	sty := newStyler(kctx.Stdout)

	wantContainers := c.JSON || c.Verbose
	var glovebox []dockerx.ContainerSummary
	if wantContainers && dc != nil {
		glovebox, err = dc.ListContainersByPrefix(context.Background(), "glovebox-")
		if err != nil && !c.JSON {
			fmt.Fprintf(kctx.Stderr, "      (failed to list containers: %v)\n", err)
		}
	}

	if c.JSON {
		return renderJSON(kctx.Stdout, activePID, projects, glovebox)
	}

	if c.Verbose {
		renderProjectTableVerbose(kctx.Stdout, sty, projects, glovebox, time.Now())
		return nil
	}

	renderProjectTable(kctx.Stdout, sty, projects)
	return nil
}

// JSON shapes for `gbx ls --json`. Field order is intentional (matches the
// natural reading order in the output); fieldalignment hints are silenced
// because reordering would swap labels (a map) and the string fields,
// degrading the human-readable JSON.
//
//nolint:govet // fieldalignment: JSON output order beats byte packing.
type jsonContainer struct {
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	ID     string            `json:"id"`
	State  string            `json:"state"`
	Status string            `json:"status"`
	Labels map[string]string `json:"labels,omitempty"`
}

type jsonProject struct {
	PID         string          `json:"pid"`
	Workspace   string          `json:"workspace"`
	AgentStatus string          `json:"agent_status"`
	StackStatus string          `json:"stack_status"`
	Containers  []jsonContainer `json:"containers"`
	Active      bool            `json:"active"`
}

type jsonRoot struct {
	ActivePID       string          `json:"active_pid,omitempty"`
	Projects        []jsonProject   `json:"projects"`
	OtherContainers []jsonContainer `json:"other_containers"`
}

// renderJSON renders the same tree the human-readable `gbx ls -v` shows, but
// as a single self-contained JSON document. `other_containers` collects
// the singleton stack and anything else in the glovebox-* pool that isn't
// claimed by a project.
func renderJSON(w io.Writer, activePID string, projects []project.Project, glovebox []dockerx.ContainerSummary) error {
	toJSONContainer := func(c dockerx.ContainerSummary) jsonContainer {
		return jsonContainer{Name: c.Name, Image: c.Image, ID: c.ID, State: c.State, Status: c.Status, Labels: c.Labels}
	}

	included := map[string]bool{}
	out := jsonRoot{ActivePID: activePID, Projects: []jsonProject{}, OtherContainers: []jsonContainer{}}
	for _, p := range projects {
		rows := containersForProject(glovebox, p.PID)
		containers := make([]jsonContainer, 0, len(rows))
		for _, r := range rows {
			included[r.Name] = true
			containers = append(containers, toJSONContainer(r))
		}
		out.Projects = append(out.Projects, jsonProject{
			PID:         p.PID,
			Workspace:   p.Workspace,
			AgentStatus: p.AgentStatus,
			StackStatus: p.StackStatus,
			Active:      p.Active,
			Containers:  containers,
		})
	}
	for _, c := range glovebox {
		if !included[c.Name] {
			out.OtherContainers = append(out.OtherContainers, toJSONContainer(c))
		}
	}
	sort.Slice(out.OtherContainers, func(i, j int) bool { return out.OtherContainers[i].Name < out.OtherContainers[j].Name })

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// containersForProject filters the caller-supplied glovebox-container pool
// down to a single project's agent + per-project stack services. The shared
// singleton stack (egress-proxy, socket-proxy, stack-controller) doesn't
// match either pattern and is excluded by construction.
func containersForProject(pool []dockerx.ContainerSummary, pid string) []dockerx.ContainerSummary {
	agentName := "glovebox-agent-" + pid
	stackPrefix := "glovebox-stack-" + pid + "-"
	var mine []dockerx.ContainerSummary
	for _, c := range pool {
		if c.Name == agentName || strings.HasPrefix(c.Name, stackPrefix) {
			mine = append(mine, c)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].Name < mine[j].Name })
	return mine
}

// renderProjectTable prints the plain (non-verbose) 4-column project table.
func renderProjectTable(w io.Writer, sty styler, projects []project.Project) {
	// fmt.Fprintf(w, "  %s  %s  %s  %s\n",
	// 	sty.bold(fmt.Sprintf("%-12s", "PROJECT-ID")),
	// 	sty.bold(fmt.Sprintf("%-65s", "WORKSPACE")),
	// 	sty.bold(fmt.Sprintf("%-8s", "AGENT")),
	// 	sty.bold("STACK"),
	// )
	for _, p := range projects {
		fmt.Fprintf(
			w,
			"%s %-12s  %-65s  %s  %s\n",
			renderProjectMarker(p.Active),
			renderProjectPID(p.Active, p.PID, sty),
			ellipsizeLeft(p.Workspace, 65),
			sty.status(p.AgentStatus, 8), sty.status(p.StackStatus, 0),
		)
	}
}

// renderProjectTableVerbose prints the `gbx ls -v` tree: each project as a node with
// its workspace and its containers as ├─/└─ leaves, followed by an OTHER
// CONTAINERS section. Container columns (name, image, status) are aligned
// globally across every project and the OTHER section. now is injected for
// deterministic tag ages in tests.
func renderProjectTableVerbose(w io.Writer, sty styler, projects []project.Project, glovebox []dockerx.ContainerSummary, now time.Time) {
	type cell struct{ name, image, status, tag string }
	toCell := func(c dockerx.ContainerSummary, name string) cell {
		return cell{
			name:   name,
			image:  ellipsizeLeft(c.Image, 30),
			status: truncRunes(c.Status, 28),
			tag:    deriveTag(c.Labels, now),
		}
	}

	type block struct {
		p     project.Project
		cells []cell
	}
	included := map[string]bool{}
	blocks := make([]block, 0, len(projects))
	for _, p := range projects {
		rows := containersForProject(glovebox, p.PID)
		cells := make([]cell, 0, len(rows))
		for _, r := range rows {
			included[r.Name] = true
			cells = append(cells, toCell(r, shortContainerName(r.Name, p.PID)))
		}
		blocks = append(blocks, block{p, cells})
	}

	var others []dockerx.ContainerSummary
	for _, c := range glovebox {
		if !included[c.Name] {
			others = append(others, c)
		}
	}
	sort.Slice(others, func(i, j int) bool { return others[i].Name < others[j].Name })
	otherCells := make([]cell, 0, len(others))
	for _, c := range others {
		name, _ := strings.CutPrefix(c.Name, "glovebox-")
		otherCells = append(otherCells, toCell(c, name))
	}

	nameW, imageW, statusW := 0, 0, 0
	measure := func(cs []cell) {
		for _, c := range cs {
			nameW = max(nameW, len(c.name))
			imageW = max(imageW, len(c.image))
			statusW = max(statusW, len(c.status))
		}
	}
	for _, b := range blocks {
		measure(b.cells)
	}
	measure(otherCells)

	printRow := func(connector string, c cell) {
		line := connector + fmt.Sprintf("%-*s  %-*s  %s",
			nameW, c.name, imageW, c.image, sty.status(c.status, statusW))
		if c.tag != "" {
			line += "  " + c.tag
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}

	fmt.Fprintln(w, sty.bold("PROJECTS"))
	for _, b := range blocks {
		fmt.Fprintf(
			w,
			"  %s %-12s   agent %s · stack %s\n",
			renderProjectMarker(b.p.Active),
			renderProjectPID(b.p.Active, b.p.PID, sty),
			sty.status(b.p.AgentStatus, 0),
			sty.status(b.p.StackStatus, 0),
		)
		fmt.Fprintf(w, "  │ %s\n", b.p.Workspace)
		if len(b.cells) == 0 {
			fmt.Fprintln(w, "  └─ "+sty.dim("(no containers)"))
			continue
		}
		for i, c := range b.cells {
			conn := "  ├─ "
			if i == len(b.cells)-1 {
				conn = "  └─ "
			}
			printRow(conn, c)
		}
	}

	if len(otherCells) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, sty.bold("OTHER CONTAINERS"))
		for _, c := range otherCells {
			printRow("  ", c)
		}
	}
}

func renderProjectMarker(active bool) string {
	if active {
		return "●"
	}
	return "○"
}

func renderProjectPID(active bool, pid string, sty styler) string {
	if active {
		return sty.bold(pid)
	}
	return pid
}

// shortContainerName strips the redundant per-project prefix from a container
// name: glovebox-agent-<pid> -> "agent", glovebox-stack-<pid>-<svc> -> "<svc>".
// Names that match neither pattern are returned unchanged.
func shortContainerName(name, pid string) string {
	if name == "glovebox-agent-"+pid {
		return "agent"
	}
	if s, ok := strings.CutPrefix(name, "glovebox-stack-"+pid+"-"); ok {
		return s
	}
	return name
}

// formatRelAge renders a coarse relative age of ts measured against now:
// "just now" (<1m), "Nm ago" (<1h), "Nh ago" (<1d), else "Nd ago".
// A future ts (clock skew) also yields "just now" - the negative duration falls into the <1m bucket.
func formatRelAge(ts, now time.Time) string {
	d := now.Sub(ts)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// deriveTag turns a container's labels into a compact display tag, replacing
// the raw LABELS column. "test" for io.glovebox.test=1; "built <age>" for the
// io.glovebox.image.created build stamp (plain "built" if it won't parse).
// Both are joined with ", " in that order; no relevant labels yields "".
func deriveTag(labels map[string]string, now time.Time) string {
	var tags []string
	if labels["io.glovebox.test"] == "1" {
		tags = append(tags, "test")
	}
	if raw, ok := labels[dockerx.ImageCreatedLabel]; ok {
		if ts, err := time.Parse(dockerx.ImageCreatedLabelFormat, raw); err == nil {
			tags = append(tags, "built "+formatRelAge(ts, now))
		} else {
			tags = append(tags, "built")
		}
	}
	return strings.Join(tags, ", ")
}
