package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/okulik/glovebox/internal/config"
)

// Server holds the two HTTP listeners.
type Server struct {
	internal *http.Server
	host     *http.Server
}

// New returns a Server with two listeners. The internal listener serves the
// agent-callable subset of routes; the host listener serves all routes.
func New(internalAddr, hostAddr string, deps Deps) *Server {
	internalMux := http.NewServeMux()
	hostMux := http.NewServeMux()

	internalDeps := applyDeps{rules: deps.Rules, docker: deps.Docker, state: deps.Store, hostOnly: false}
	hostDeps := applyDeps{rules: deps.Rules, docker: deps.Docker, state: deps.Store, hostOnly: true}

	registerCommon(internalMux, internalDeps)
	registerCommon(hostMux, hostDeps)
	registerHostOnly(hostMux, hostDeps)

	return &Server{
		internal: &http.Server{Addr: internalAddr, Handler: logRequests("internal", internalMux), ReadHeaderTimeout: 5 * time.Second},
		host:     &http.Server{Addr: hostAddr, Handler: logRequests("host", hostMux), ReadHeaderTimeout: 5 * time.Second},
	}
}

// logRequests wraps http.Handler to emit one log line per request (label, method, path,
// status, duration). The controller's stdout/stderr is what `gbx logs
// controller` follows, so this is how in-container `gbx-stack` calls (label
// "internal") and host `gbx stack` calls (label "host") become visible.
// /health is skipped: the container HEALTHCHECK hits it on a short interval
// and would otherwise drown out real traffic.
func logRequests(label string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == config.HealthPath {
			h.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		//nolint:gosec // G706: %q escapes control chars (e.g. CRLF) in the tainted method/path, so a crafted request can't forge log lines.
		log.Printf("%s %q %q %d %s", label, r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder captures the response status code for request logging while
// delegating everything else to the wrapped ResponseWriter.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func registerCommon(mux *http.ServeMux, deps applyDeps) {
	mux.HandleFunc("GET "+config.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /projects/{pid}/status", newStatusHandler(deps))
	mux.Handle("POST /projects/{pid}/services/{svc}/start", newServiceHandler(deps, "start"))
	mux.Handle("POST /projects/{pid}/services/{svc}/stop", newServiceHandler(deps, "stop"))
	mux.Handle("POST /projects/{pid}/services/{svc}/reset", newResetHandler(deps))
	mux.Handle("GET /projects/{pid}/info", newInfoHandler(deps))
	mux.Handle("GET /projects/{pid}/services/{svc}/logs", newLogsHandler(deps))
	mux.Handle("POST /projects/{pid}/propose", newProposeHandler(deps))
	mux.Handle("GET /projects/{pid}/manifests", newManifestsHandler(deps))
}

func registerHostOnly(mux *http.ServeMux, deps applyDeps) {
	mux.Handle("POST /projects/{pid}/apply", newApplyHandler(deps))
	mux.Handle("POST /projects/{pid}/down", newDownHandler(deps))
	mux.Handle("POST /projects/{pid}/destroy", newDestroyHandler(deps))
	mux.Handle("GET /projects", newListProjectsHandler(deps))
}

// Start runs both listeners in goroutines. Returns the first error from either.
func (s *Server) Start() error {
	errCh := make(chan error, 2)
	go func() { errCh <- s.internal.ListenAndServe() }()
	go func() { errCh <- s.host.ListenAndServe() }()
	err := <-errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops both listeners.
func (s *Server) Shutdown(ctx context.Context) error {
	_ = s.internal.Shutdown(ctx)
	return s.host.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Header is already flushed at this point so the response is partial;
		// surface the encode failure to logs so it isn't silently swallowed.
		log.Printf("writeJSON encode error: %v", err)
	}
}
