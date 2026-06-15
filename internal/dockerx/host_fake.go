package dockerx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// FakeHost records every Host call for assertion in tests. Each call appends
// to Calls in the format `verb arg1 arg2 …`. Default response is success;
// inject per-call errors via *Err fields or per-image existence via Images.
//
// FakeHost is not safe for concurrent use - drive it from a single test
// goroutine. Tests that need parallelism should embed their own mutex.
// FakeHost field order favors readability (related fields grouped) over the
// 8-byte memory saving fieldalignment suggests; this is a test helper, not
// hot-path data, so the trade is fine.
//
//nolint:govet // fieldalignment: 8-byte savings irrelevant for a test fake.
type FakeHost struct {
	LastExec   ExecSpec
	LastBuild  BuildSpec
	BuildErr   error
	StopErr    error
	RestartErr error
	RemoveErr  error
	ExecErr    error
	Images     map[string]bool
	Calls      []string
	LogOutput  string
	LogErr     error
	DaemonDown bool
}

// NewFakeHost returns a fresh FakeHost.
func NewFakeHost() *FakeHost {
	return &FakeHost{Images: map[string]bool{}}
}

func (f *FakeHost) record(format string, args ...any) {
	f.Calls = append(f.Calls, fmt.Sprintf(format, args...))
}

func (f *FakeHost) DaemonReachable(_ context.Context) error {
	f.record("daemon-reachable")
	if f.DaemonDown {
		return errors.New("fake: daemon not reachable")
	}
	return nil
}

func (f *FakeHost) ImageExists(_ context.Context, image string) bool {
	f.record("image-exists %s", image)
	return f.Images[image]
}

func (f *FakeHost) BuildImage(_ context.Context, spec BuildSpec) error {
	f.record("build %s -f %s ctx=%s", spec.Tag, spec.Dockerfile, spec.Context)
	f.LastBuild = spec
	// Pretend the build succeeded by marking the image present.
	if f.Images == nil {
		f.Images = map[string]bool{}
	}
	f.Images[spec.Tag] = true
	return f.BuildErr
}

func (f *FakeHost) StopContainer(_ context.Context, name string) error {
	f.record("stop %s", name)
	return f.StopErr
}

func (f *FakeHost) RestartContainer(_ context.Context, name string) error {
	f.record("restart %s", name)
	return f.RestartErr
}

func (f *FakeHost) RemoveImage(_ context.Context, image string) error {
	f.record("rmi %s", image)
	delete(f.Images, image)
	return f.RemoveErr
}

func (f *FakeHost) ForceRemoveContainer(_ context.Context, name string) error {
	f.record("rm -f %s", name)
	return f.RemoveErr
}

func (f *FakeHost) ContainerLogs(_ context.Context, name string, tail int, follow bool, stdout, _ io.Writer) error {
	f.record("logs %s tail=%d follow=%v", name, tail, follow)
	if f.LogOutput != "" && stdout != nil {
		_, _ = io.Copy(stdout, bytes.NewReader([]byte(f.LogOutput)))
	}
	return f.LogErr
}

func (f *FakeHost) Exec(_ context.Context, spec ExecSpec) error {
	f.record("exec %s argv=%v", spec.Container, spec.Argv)
	f.LastExec = spec
	// Drain any provided stdin into a buffer so callers can assert it later.
	if spec.In != nil {
		_, _ = io.Copy(&bytes.Buffer{}, spec.In)
	}
	return f.ExecErr
}
