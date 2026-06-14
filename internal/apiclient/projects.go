package apiclient

import (
	"context"
	"fmt"
	"io"
)

// Manifests GETs /projects/<pid>/manifests, returning a JSON body of the form
// {"live": <yaml|null>, "proposed": <yaml|null>}.
func (c *Client) Manifests(ctx context.Context, pid string) ([]byte, int, error) {
	return c.do(ctx, "GET", fmt.Sprintf("/projects/%s/manifests", pid), nil, "")
}

// Apply POSTs to /projects/<pid>/apply. A non-nil manifest body uploads and
// applies that manifest; a nil body tells the controller to apply the project's
// stored proposal. The controller responds with a JSON body (2xx on success).
func (c *Client) Apply(ctx context.Context, pid string, manifest io.Reader) ([]byte, int, error) {
	return c.do(ctx, "POST", fmt.Sprintf("/projects/%s/apply", pid), manifest, "text/yaml")
}

// Down POSTs to /projects/<pid>/down (stop services, keep volumes).
func (c *Client) Down(ctx context.Context, pid string) ([]byte, int, error) {
	return c.do(ctx, "POST", fmt.Sprintf("/projects/%s/down", pid), nil, "")
}

// Destroy POSTs to /projects/<pid>/destroy?confirm=true. The confirm query
// param is required by the controller; bash always sends it.
func (c *Client) Destroy(ctx context.Context, pid string) ([]byte, int, error) {
	return c.do(ctx, "POST", fmt.Sprintf("/projects/%s/destroy?confirm=true", pid), nil, "")
}

// Status GETs /projects/<pid>/status.
func (c *Client) Status(ctx context.Context, pid string) ([]byte, int, error) {
	return c.do(ctx, "GET", fmt.Sprintf("/projects/%s/status", pid), nil, "")
}

// ListProjects GETs /projects.
func (c *Client) ListProjects(ctx context.Context) ([]byte, int, error) {
	return c.do(ctx, "GET", "/projects", nil, "")
}

// Logs GETs /projects/<pid>/services/<svc>/logs (optionally with ?follow=true).
func (c *Client) Logs(ctx context.Context, pid, svc string, follow bool) ([]byte, int, error) {
	path := fmt.Sprintf("/projects/%s/services/%s/logs", pid, svc)
	if follow {
		path += "?follow=true"
	}
	return c.do(ctx, "GET", path, nil, "")
}
