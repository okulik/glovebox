package apiclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recorder struct {
	method, path, query string
}

func newServer(t *testing.T, r *recorder, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.method = req.Method
		r.path = req.URL.Path
		r.query = req.URL.RawQuery
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestApply(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 202, `{"status":"applied"}`)
	defer srv.Close()
	body, code, err := New(srv.URL).Apply(t.Context(), "hostapply", strings.NewReader("version: 1\n"))
	if err != nil || code != 202 || string(body) != `{"status":"applied"}` {
		t.Fatalf("got code=%d err=%v body=%s", code, err, body)
	}
	if r.method != "POST" || r.path != "/projects/hostapply/apply" {
		t.Errorf("unexpected request: %s %s", r.method, r.path)
	}
}

func TestDown(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, `{}`)
	defer srv.Close()
	_, code, err := New(srv.URL).Down(t.Context(), "p1")
	if err != nil || code != 200 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if r.method != "POST" || r.path != "/projects/p1/down" {
		t.Errorf("unexpected: %s %s", r.method, r.path)
	}
}

func TestDestroyAlwaysSendsConfirm(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, `{}`)
	defer srv.Close()
	_, _, err := New(srv.URL).Destroy(t.Context(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if r.method != "POST" || r.path != "/projects/p1/destroy" {
		t.Errorf("unexpected: %s %s", r.method, r.path)
	}
	if r.query != "confirm=true" {
		t.Errorf("destroy must send ?confirm=true, got %q", r.query)
	}
}

func TestStatus(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, `{"state":"ready"}`)
	defer srv.Close()
	body, _, err := New(srv.URL).Status(t.Context(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"state":"ready"}` {
		t.Errorf("body: %s", body)
	}
	if r.method != "GET" || r.path != "/projects/p1/status" {
		t.Errorf("unexpected: %s %s", r.method, r.path)
	}
}

func TestListProjects(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, `["p1","p2"]`)
	defer srv.Close()
	body, _, err := New(srv.URL).ListProjects(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `["p1","p2"]` {
		t.Errorf("body: %s", body)
	}
	if r.method != "GET" || r.path != "/projects" {
		t.Errorf("unexpected: %s %s", r.method, r.path)
	}
}

func TestLogsWithFollow(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, "line1\nline2\n")
	defer srv.Close()
	body, _, err := New(srv.URL).Logs(t.Context(), "p1", "redis", true)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "line1\nline2\n" {
		t.Errorf("body: %q", body)
	}
	if r.path != "/projects/p1/services/redis/logs" {
		t.Errorf("path: %s", r.path)
	}
	if r.query != "follow=true" {
		t.Errorf("follow query: %q", r.query)
	}
}

func TestLogsWithoutFollow(t *testing.T) {
	var r recorder
	srv := newServer(t, &r, 200, "")
	defer srv.Close()
	_, _, err := New(srv.URL).Logs(t.Context(), "p1", "redis", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.query != "" {
		t.Errorf("logs(follow=false) should have no query, got %q", r.query)
	}
}
