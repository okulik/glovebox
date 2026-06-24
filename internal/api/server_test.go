package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog redirects the standard logger to a buffer for the duration of the
// test and returns it.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

func TestLogRequestsEmitsLine(t *testing.T) {
	buf := captureLog(t)
	h := logRequests("internal", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/projects/abc123/status", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	got := buf.String()
	for _, want := range []string{"internal", http.MethodGet, "/projects/abc123/status", "418"} {
		if !strings.Contains(got, want) {
			t.Errorf("request log missing %q: %q", want, got)
		}
	}
}

func TestLogRequestsSkipsHealth(t *testing.T) {
	buf := captureLog(t)
	served := false
	h := logRequests("internal", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !served || rec.Code != http.StatusOK {
		t.Errorf("/health must still be served: served=%v code=%d", served, rec.Code)
	}
	if got := buf.String(); got != "" {
		t.Errorf("/health should not be logged (HEALTHCHECK noise), got %q", got)
	}
}
