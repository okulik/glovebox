package main

import (
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

// withFakeDocker injects fake host + controller clients as the package-level
// docker handles and sets GBX_LIBEXEC (stack.FromEnv requires it), restoring
// everything on cleanup.
func withFakeDocker(t *testing.T) *dockerx.FakeHost {
	t.Helper()
	fh := dockerx.NewFakeHost()
	hostDocker = fh
	hostClient = dockerx.NewFake()
	t.Setenv(config.EnvLibexec, t.TempDir())
	t.Cleanup(func() { hostDocker = nil; hostClient = nil })
	return fh
}

func TestLogsControllerStreamsControllerContainer(t *testing.T) {
	fh := withFakeDocker(t)
	_, stderr, code := runCLI(t, "logs", "controller")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	joined := strings.Join(fh.Calls, "\n")
	if !strings.Contains(joined, "logs glovebox-stack-controller") {
		t.Errorf("expected a controller logs call, got calls: %v", fh.Calls)
	}
}

func TestLogsProxyStreamsEgressProxy(t *testing.T) {
	fh := withFakeDocker(t)
	_, stderr, code := runCLI(t, "logs", "proxy")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	joined := strings.Join(fh.Calls, "\n")
	if !strings.Contains(joined, "exec glovebox-egress-proxy") {
		t.Errorf("expected an egress-proxy exec call, got calls: %v", fh.Calls)
	}
}

func TestLogsUnknownTargetErrors(t *testing.T) {
	withFakeDocker(t)
	_, stderr, code := runCLI(t, "logs", "bogus")
	if code == 0 {
		t.Fatal("want non-zero exit for unknown target")
	}
	if !strings.Contains(stderr, "proxy") || !strings.Contains(stderr, "controller") {
		t.Errorf("usage error should name both targets, got %q", stderr)
	}
}
