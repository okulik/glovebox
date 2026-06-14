// Package apiclient is the host-side HTTP client for the glovebox
// stack-controller. It exposes one method per controller endpoint that the
// gbx host CLI calls; lower-level access via Client.do for tests.
package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single controller HTTP base URL.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// New returns a Client with a default 30-second timeout. Callers may swap
// HTTP for one with a different timeout (e.g. logs streaming).
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// do issues a single HTTP request and returns the response body + status
// code. Non-2xx responses are NOT treated as errors - the caller (CLI layer)
// decides what to do with the status. err is only set for network or
// request-building failures.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return data, resp.StatusCode, nil
}
