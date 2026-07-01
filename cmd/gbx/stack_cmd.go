package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/apiclient"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/state"
)

// StackCmd is the `gbx stack ...` command group.
type StackCmd struct {
	Diff       StackDiffCmd       `cmd:"" help:"Show live vs proposed for a project."`
	Ls         StackLsCmd         `cmd:"" help:"List all projects with stacks."`
	Down       StackDownCmd       `cmd:"" help:"Stop services, keep volumes."`
	Status     StackStatusCmd     `cmd:"" help:"Show service health."`
	ImageAllow StackImageAllowCmd `cmd:"" name:"image-allow" help:"Append a registry to docker/image-allowlist.txt."`
	Logs       StackLogsCmd       `cmd:"" help:"Stream service logs."`
	Apply      StackApplyCmd      `cmd:"" help:"Apply the project's stored proposal."`
	Destroy    StackDestroyCmd    `cmd:"" help:"Stop + remove services + volumes."`
}

// stackPID resolves the project for stack subcommands, in priority order:
// -p/--pid (GBX_OVERRIDE_PID) > GBX_PROJECT_ID > active project. The global
// -p/--pid flag (see CLI.PFlag) feeds GBX_OVERRIDE_PID, so stack commands
// share the same pid selector as the rest of the CLI. Returns an actionable
// error when none is set, rather than defaulting to the literal "default".
func stackPID() (string, error) {
	if p := os.Getenv(config.EnvOverridePID); p != "" {
		return p, nil
	}
	if p := os.Getenv(config.EnvProjectID); p != "" {
		return p, nil
	}
	pid, err := state.ActivePID(config.GbxFromEnv().ConfigDir)
	if err != nil {
		return "", err
	}
	if pid == "" {
		return "", errors.New("no project selected; pass -p/--pid <id>, or run 'gbx use <id>'")
	}
	return pid, nil
}

func controllerURL() string {
	return config.GbxFromEnv().ControllerURL
}

// Ls / Status / Down

type StackLsCmd struct{}

func (c *StackLsCmd) Run(kctx *kong.Context) error {
	body, code, err := apiclient.New(controllerURL()).ListProjects(context.Background())
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d: %s", code, body)
	}
	fmt.Fprintln(kctx.Stdout, string(body))
	return nil
}

type StackStatusCmd struct{}

func (c *StackStatusCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	body, code, err := apiclient.New(controllerURL()).Status(context.Background(), pid)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d: %s", code, body)
	}
	fmt.Fprintln(kctx.Stdout, string(body))
	return nil
}

type StackDownCmd struct{}

func (c *StackDownCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	body, code, err := apiclient.New(controllerURL()).Down(context.Background(), pid)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d: %s", code, body)
	}
	fmt.Fprintln(kctx.Stdout, string(body))
	return nil
}

type StackApplyCmd struct {
	Yes    bool `short:"y" name:"yes" help:"Skip prompt."`
	DryRun bool `name:"dry-run" help:"Show diff/manifest without POSTing."`
}

func (c *StackApplyCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	client := apiclient.New(controllerURL())

	if c.DryRun {
		mBody, mCode, mErr := client.Manifests(context.Background(), pid)
		if mErr != nil {
			return mErr
		}
		if mCode < 200 || mCode >= 300 {
			return fmt.Errorf("controller returned %d: %s", mCode, mBody)
		}
		var resp struct {
			Proposed *string `json:"proposed"`
		}
		if mErr = json.Unmarshal(mBody, &resp); mErr != nil {
			return fmt.Errorf("decode manifests: %w", mErr)
		}
		if resp.Proposed == nil {
			return fmt.Errorf("no proposal to apply for project %s", pid)
		}
		fmt.Fprint(kctx.Stdout, *resp.Proposed)
		return nil
	}

	if !c.Yes && !confirmYN(kctx.Stderr, fmt.Sprintf("Apply stored proposal for project %s? [y/N] ", pid)) {
		return errors.New("Aborted.")
	}

	// Empty body ⇒ controller applies the stored proposal and clears it.
	body, code, err := client.Apply(context.Background(), pid, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(kctx.Stdout, string(body))
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d", code)
	}
	return nil
}

type StackDestroyCmd struct {
	Yes bool `short:"y" name:"yes" help:"Skip prompt."`
}

func (c *StackDestroyCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	if !c.Yes && !confirmYN(kctx.Stderr, fmt.Sprintf("Destroy stack for project %s (removes services + volumes)? [y/N] ", pid)) {
		return errors.New("Aborted.")
	}
	body, code, err := apiclient.New(controllerURL()).Destroy(context.Background(), pid)
	if err != nil {
		return err
	}
	fmt.Fprintln(kctx.Stdout, string(body))
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d", code)
	}
	return nil
}

type StackDiffCmd struct{}

func (c *StackDiffCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	body, code, err := apiclient.New(controllerURL()).Manifests(context.Background(), pid)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d: %s", code, body)
	}
	var resp struct {
		Live     *string `json:"live"`
		Proposed *string `json:"proposed"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode manifests: %w", err)
	}
	switch {
	case resp.Live == nil && resp.Proposed == nil:
		fmt.Fprintf(kctx.Stdout, "no manifests for project %s\n", pid)
	case resp.Proposed == nil:
		fmt.Fprintln(kctx.Stdout, "(no pending proposal; showing live manifest)")
		fmt.Fprint(kctx.Stdout, *resp.Live)
	case resp.Live == nil:
		fmt.Fprintln(kctx.Stdout, "(no live manifest)")
		fmt.Fprint(kctx.Stdout, *resp.Proposed)
	default:
		fmt.Fprintf(kctx.Stdout, "--- live\n+++ proposed\n")
		fmt.Fprint(kctx.Stdout, simpleDiff(*resp.Live, *resp.Proposed))
	}
	return nil
}

func simpleDiff(a, b string) string {
	aLines := strings.Split(strings.TrimRight(a, "\n"), "\n")
	bLines := strings.Split(strings.TrimRight(b, "\n"), "\n")
	aSet := map[string]bool{}
	for _, l := range aLines {
		aSet[l] = true
	}
	bSet := map[string]bool{}
	for _, l := range bLines {
		bSet[l] = true
	}
	var out strings.Builder
	for _, l := range aLines {
		if bSet[l] {
			out.WriteString(" " + l + "\n")
		} else {
			out.WriteString("-" + l + "\n")
		}
	}
	for _, l := range bLines {
		if !aSet[l] {
			out.WriteString("+" + l + "\n")
		}
	}
	return out.String()
}

type StackLogsCmd struct {
	Service string `arg:"" help:"Service name."`
	Follow  bool   `name:"follow" help:"Follow log output."`
}

func (c *StackLogsCmd) Run(kctx *kong.Context) error {
	pid, err := stackPID()
	if err != nil {
		return err
	}
	body, code, err := apiclient.New(controllerURL()).Logs(context.Background(), pid, c.Service, c.Follow)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("controller returned %d: %s", code, body)
	}
	fmt.Fprint(kctx.Stdout, string(body))
	return nil
}

type StackImageAllowCmd struct {
	Registry string `arg:"" help:"Registry to append to docker/image-allowlist.txt."`
}

func (c *StackImageAllowCmd) Run(kctx *kong.Context) error {
	libexec := os.Getenv(config.EnvLibexec)
	if libexec == "" {
		return errors.New("GBX_LIBEXEC not set")
	}
	// Containment: confine the allowlist file to <libexec>/docker/, since
	// libexec is taint-derived (read from GBX_LIBEXEC env).
	path, err := agent.UnderBase(
		filepath.Join(libexec, config.DockerDirName),
		filepath.Join(libexec, config.DockerDirName, "image-allowlist.txt"),
	)
	if err != nil {
		return err
	}
	//nolint:gosec // G703: path is validated by agent.UnderBase above
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("allowlist not found at %s: %w", path, err)
	}
	if slices.Contains(strings.Split(string(data), "\n"), c.Registry) {
		fmt.Fprintf(kctx.Stdout, "already allowed: %s\n", c.Registry)
		return nil
	}
	//nolint:gosec // G703: path is validated by agent.UnderBase above
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(c.Registry + "\n"); err != nil {
		return err
	}
	fmt.Fprintf(kctx.Stdout, "appended: %s\n", c.Registry)
	fmt.Fprintln(kctx.Stdout, "(restart the controller to pick up the new entry: gbx restart controller)")
	return nil
}
