package apiclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/projects" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	body, code, err := c.do(t.Context(), "GET", "/projects", nil, "")
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if code != 200 || string(body) != `{"ok":true}` {
		t.Fatalf("got code=%d body=%s", code, body)
	}
}

func TestDoPostWithBody(t *testing.T) {
	var gotBody string
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	body, code, err := c.do(t.Context(), "POST", "/projects/p1/apply",
		strings.NewReader("version: 1\n"), "text/yaml")
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if code != 202 || string(body) != `{"status":"accepted"}` {
		t.Fatalf("got code=%d body=%s", code, body)
	}
	if gotBody != "version: 1\n" {
		t.Fatalf("server saw body %q", gotBody)
	}
	if gotCT != "text/yaml" {
		t.Fatalf("server saw content-type %q", gotCT)
	}
}

func TestDoNetworkError(t *testing.T) {
	c := New("http://127.0.0.1:1") // unlikely to be listening
	_, _, err := c.do(t.Context(), "GET", "/anything", nil, "")
	if err == nil {
		t.Fatal("want error for unreachable URL")
	}
}
