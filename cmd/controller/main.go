package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/okulik/glovebox/internal/api"
	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
	"github.com/okulik/glovebox/internal/manifest"
	"github.com/okulik/glovebox/internal/state"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false,
		"probe http://localhost<InternalAddr>/health and exit 0 (healthy) or 1 (unhealthy). Used as the container HEALTHCHECK CMD since the distroless image has no shell or wget.")
	flag.Parse()

	cfg := config.ControllerFromEnv()

	if *healthcheck {
		os.Exit(runHealthcheck(cfg))
	}

	log.Printf("stack-controller starting: internal=%s host=%s docker=%s",
		cfg.InternalAddr, cfg.HostAddr, cfg.DockerHost)

	rules, err := manifest.LoadRules(cfg.ImageAllowlistPath, 4, 8<<30)
	if err != nil {
		log.Fatalf("load rules: %v", err)
	}
	dock, err := dockerx.NewControllerClient(cfg.DockerHost)
	if err != nil {
		log.Fatalf("docker client: %v", err)
	}
	store, err := state.Open(filepath.Join(cfg.StateDir, "projects.json"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	deps := api.Deps{Rules: rules, Docker: dock, Store: store}

	log.Printf("reconciling state...")
	if err := state.Reconcile(context.Background(), store, dock); err != nil {
		log.Printf("reconcile error (non-fatal): %v", err)
	}

	srv := api.New(cfg.InternalAddr, cfg.HostAddr, deps)

	done := make(chan error, 1)
	go func() { done <- srv.Start() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("failed to shutdown controller: %v", err)
		}
	case err := <-done:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	}
}

// runHealthcheck performs the in-container Docker HEALTHCHECK: dial our
// own internal listener on loopback and GET /health. Returns 0 on 2xx, 1
// on anything else (transport error, non-2xx, or timeout). We probe the
// internal address rather than the host one because it's the same process
// and the latter is host-loopback-bound.
func runHealthcheck(cfg config.ControllerConfig) int {
	addr := cfg.InternalAddr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+config.HealthPath, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 1
	}
	return 0
}
