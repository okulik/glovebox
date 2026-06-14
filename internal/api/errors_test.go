package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okulik/glovebox/internal/manifest"
)

func TestWriteValidationError_Shape(t *testing.T) {
	w := httptest.NewRecorder()
	verr := &manifest.ValidationError{
		Code: "image_registry_not_allowed", Path: "services.x.image",
		Message: "blah", HintForAgent: "use docker.io",
	}
	writeValidationError(w, verr)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "image_registry_not_allowed" {
		t.Errorf("error code in `error` key = %q, want image_registry_not_allowed; body = %v", body["error"], body)
	}
	if body["path"] != "services.x.image" {
		t.Errorf("path = %q; body = %v", body["path"], body)
	}
	if body["hint_for_agent"] != "use docker.io" {
		t.Errorf("hint_for_agent = %q; body = %v", body["hint_for_agent"], body)
	}
}

func TestWriteError_Shape(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "image_pull_failed", "boom")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "image_pull_failed" || body["message"] != "boom" {
		t.Errorf("body = %v", body)
	}
}
