package api

import (
	"net/http"

	"github.com/okulik/glovebox/internal/manifest"
)

// APIError is the canonical error response shape. Every error-writing path
// produces this; clients see one schema regardless of where the error came
// from. The top-level JSON key is "error" (the machine-readable code, e.g.
// "image_pull_failed" or "image_registry_not_allowed"); message is the
// human-readable detail; path and hint_for_agent are present for validation
// errors that carry that extra context.
type APIError struct {
	Code         string `json:"error"`
	Message      string `json:"message,omitempty"`
	Path         string `json:"path,omitempty"`
	HintForAgent string `json:"hint_for_agent,omitempty"`
}

// writeAPIError emits e at the given HTTP status.
func writeAPIError(w http.ResponseWriter, status int, e APIError) {
	writeJSON(w, status, e)
}

// writeError is the common case: just a code and message.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeAPIError(w, status, APIError{Code: code, Message: msg})
}

// writeValidationError surfaces a manifest.ValidationError to the client with
// the same APIError shape as every other error path.
func writeValidationError(w http.ResponseWriter, e *manifest.ValidationError) {
	writeAPIError(w, http.StatusBadRequest, APIError{
		Code:         e.Code,
		Message:      e.Message,
		Path:         e.Path,
		HintForAgent: e.HintForAgent,
	})
}
