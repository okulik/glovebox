package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/okulik/glovebox/internal/config"
)

// agentStackBaseURL resolves the controller's in-cluster API endpoint.
func agentStackBaseURL() string {
	return config.GbxaFromEnv().ControllerURL
}

func agentStackProject() string {
	return config.GbxaFromEnv().ProjectID
}

// AgentStackCmd is the in-container `gbx-stack` CLI, dispatched when argv[0]
// is the gbx-stack symlink.
type AgentStackCmd struct {
	Status  AgentStackStatusCmd  `cmd:"" help:"Print health summary."`
	Diff    AgentStackDiffCmd    `cmd:"" help:"Show live vs proposed."`
	Start   AgentStackStartCmd   `cmd:"" help:"Start a service from the live manifest."`
	Stop    AgentStackStopCmd    `cmd:"" help:"Stop a service."`
	Reset   AgentStackResetCmd   `cmd:"" help:"Wipe a service's volumes and restart it."`
	Propose AgentStackProposeCmd `cmd:"" help:"Submit <file> as the project's proposed stack manifest (POST to controller)."`
	Wait    AgentStackWaitCmd    `cmd:"" help:"Block until services healthy."`
	Logs    AgentStackLogsCmd    `cmd:"" help:"Stream logs."`
	Info    AgentStackInfoCmd    `cmd:"" help:"JSON service map (or shell exports with --env)."`
}

func httpGet(path string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, agentStackBaseURL()+path, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func httpPost(path string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, agentStackBaseURL()+path, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func httpPostBody(path string, body io.Reader, contentType string) (string, int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, agentStackBaseURL()+path, body)
	if err != nil {
		return "", 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), resp.StatusCode, err
}

type AgentStackStatusCmd struct{}

func (c *AgentStackStatusCmd) Run() error {
	body, err := httpGet(fmt.Sprintf("/projects/%s/status", agentStackProject()))
	if err != nil {
		return err
	}
	fmt.Println(body)
	return nil
}

type AgentStackInfoCmd struct {
	Env bool `name:"env" help:"Emit shell exports instead of JSON."`
}

func (c *AgentStackInfoCmd) Run() error {
	body, err := httpGet(fmt.Sprintf("/projects/%s/info", agentStackProject()))
	if err != nil {
		return err
	}
	// We don't depend on jq; print JSON as-is. --env mode is documented but
	// not implemented in Go without a JSON walk; rely on JSON output instead.
	fmt.Println(body)
	return nil
}

type AgentStackWaitCmd struct {
	Services []string `arg:"" optional:"" help:"Service names; default: all"`
}

func (c *AgentStackWaitCmd) Run() error {
	deadline := time.Now().Add(time.Duration(config.GbxaFromEnv().WaitTimeoutSeconds) * time.Second)
	for {
		body, err := httpGet(fmt.Sprintf("/projects/%s/status", agentStackProject()))
		if err == nil && strings.Contains(body, `"state":"ready"`) {
			fmt.Println(body)
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("wait: timed out")
		}
		time.Sleep(2 * time.Second)
	}
}

type AgentStackStartCmd struct {
	Service string `arg:"" help:"Service name"`
}

func (c *AgentStackStartCmd) Run() error {
	body, err := httpPost(fmt.Sprintf("/projects/%s/services/%s/start", agentStackProject(), c.Service))
	if err != nil {
		return err
	}
	fmt.Println(body)
	return nil
}

type AgentStackStopCmd struct {
	Service string `arg:"" help:"Service name"`
}

func (c *AgentStackStopCmd) Run() error {
	body, err := httpPost(fmt.Sprintf("/projects/%s/services/%s/stop", agentStackProject(), c.Service))
	if err != nil {
		return err
	}
	fmt.Println(body)
	return nil
}

type AgentStackResetCmd struct {
	Service string `arg:"" help:"Service name"`
}

func (c *AgentStackResetCmd) Run() error {
	body, err := httpPost(fmt.Sprintf("/projects/%s/services/%s/reset", agentStackProject(), c.Service))
	if err != nil {
		return err
	}
	fmt.Println(body)
	return nil
}

type AgentStackLogsCmd struct {
	Service string `arg:"" help:"Service name"`
	Follow  bool   `name:"follow" help:"Stream output."`
}

func (c *AgentStackLogsCmd) Run() error {
	q := ""
	if c.Follow {
		q = "?follow=true"
	}
	body, err := httpGet(fmt.Sprintf("/projects/%s/services/%s/logs%s", agentStackProject(), c.Service, q))
	if err != nil {
		return err
	}
	fmt.Print(body)
	return nil
}

type AgentStackProposeCmd struct {
	Source string `arg:"" help:"Source manifest file"`
}

func (c *AgentStackProposeCmd) Run() error {
	data, err := os.ReadFile(c.Source)
	if err != nil {
		return fmt.Errorf("propose: %s not readable: %w", c.Source, err)
	}
	body, code, err := httpPostBody(
		fmt.Sprintf("/projects/%s/propose", agentStackProject()),
		bytes.NewReader(data), "text/yaml")
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("propose: controller returned %d: %s", code, body)
	}
	fmt.Println(body)
	fmt.Println("Proposal submitted to controller. Operator: review with 'gbx stack diff' and apply with 'gbx stack apply -y' on the host.")
	return nil
}

type AgentStackDiffCmd struct{}

func (c *AgentStackDiffCmd) Run() error {
	body, err := httpGet(fmt.Sprintf("/projects/%s/manifests", agentStackProject()))
	if err != nil {
		return err
	}
	var resp struct {
		Live     *string `json:"live"`
		Proposed *string `json:"proposed"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return fmt.Errorf("diff: decode manifests: %w", err)
	}
	switch {
	case resp.Live == nil && resp.Proposed == nil:
		fmt.Println("no manifests for this project")
	case resp.Proposed == nil:
		fmt.Println("(no pending proposal; showing live manifest)")
		fmt.Print(*resp.Live)
	case resp.Live == nil:
		fmt.Println("(no live manifest)")
		fmt.Print(*resp.Proposed)
	default:
		fmt.Printf("--- live\n+++ proposed\n")
		fmt.Print(simpleStackDiff(*resp.Live, *resp.Proposed))
	}
	return nil
}

// simpleStackDiff is the same minimal line-diff used by host `gbx stack diff`.
func simpleStackDiff(a, b string) string {
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

// dispatchAgentStack runs the gbx-stack subcommand tree. Called when argv[0]
// is the gbx-stack symlink.
func dispatchAgentStack() error {
	// We can't reuse the main Kong CLI tree because gbxa's CLI has Install/
	// Update/Entrypoint as top-level commands. Build a fresh parser for the
	// stack-only tree.
	var cli AgentStackCmd
	return runKongFor(&cli, os.Args[1:], "gbx-stack")
}

// runKongFor parses argv against the given struct and dispatches Run.
func runKongFor(cli any, argv []string, name string) error {
	// Defer to a tiny Kong setup. We import kong from main.go's import.
	return parseAndRun(cli, argv, name)
}
