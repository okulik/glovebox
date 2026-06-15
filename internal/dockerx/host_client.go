package dockerx

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	dockerclient "github.com/moby/moby/client"
	"golang.org/x/term"
)

const (
	// ImageCreatedLabel is stamped on every image BuildImage produces, with the
	// build time as its value. Containers inherit image labels, so `gbx ls -v`
	// shows which build a container's image came from - the tag alone can't tell
	// (each rebuild moves `glovebox-agent:local` to the new image, leaving older
	// containers on a now-untagged one).
	ImageCreatedLabel = "io.glovebox.image.created"

	// ImageCreatedLabelFormat renders ISO-8601 UTC with millisecond precision,
	// e.g. 2024-09-09T14:16:46.786Z.
	ImageCreatedLabelFormat = "2006-01-02T15:04:05.000Z"
)

// HostClient is the subset of `docker` CLI invocations the host-side gbx makes.
// Production uses NewHost (shells out via os/exec). Tests pass a fake.
//
// This interface deliberately stays narrow: each method maps to exactly one
// invocation pattern of the docker CLI. We don't try to recreate the docker
// SDK - for streamed commands like `build` and `exec`, shelling out keeps
// the user-visible output intact with one line of glue.
type HostClient interface {
	// DaemonReachable returns nil if `docker info` succeeds.
	DaemonReachable(ctx context.Context) error

	// ImageExists reports whether image is present locally. Returns false
	// (with nil error) when the image is missing or the daemon is down -
	// callers use this as a probe before deciding to build.
	ImageExists(ctx context.Context, image string) bool

	// BuildImage runs `docker build` with streaming stdout/stderr.
	BuildImage(ctx context.Context, spec BuildSpec) error

	// StopContainer runs `docker stop <name>`.
	StopContainer(ctx context.Context, name string) error

	// RestartContainer runs `docker restart <name>`.
	RestartContainer(ctx context.Context, name string) error

	// RemoveImage runs the equivalent of `docker rmi -f <image>`. A missing
	// image is not treated as an error.
	RemoveImage(ctx context.Context, image string) error

	// ForceRemoveContainer runs `docker rm -f <name>`. A missing container
	// is not treated as an error.
	ForceRemoveContainer(ctx context.Context, name string) error

	// Exec runs `docker exec` with optional TTY, user, and workdir, wiring
	// the caller-provided In/Out/Err streams (defaults to os.Std* when nil).
	// On exit code != 0 the returned error is an *exec.ExitError; the
	// docker CLI's own stderr appears on Err.
	Exec(ctx context.Context, spec ExecSpec) error

	// ContainerLogs streams a container's stdout/stderr to the given writers,
	// demuxing Docker's multiplexed log stream (the controller is a non-TTY
	// container, so its frames carry stdcopy headers). tail caps the past
	// backlog (0 = all); follow keeps streaming until ctx is canceled.
	// Writers default to os.Stdout/os.Stderr when nil.
	ContainerLogs(ctx context.Context, name string, tail int, follow bool, stdout, stderr io.Writer) error
}

// BuildSpec is the input to Host.BuildImage. Args become repeated
// `--build-arg KEY=VALUE` flags.
type BuildSpec struct {
	Out        io.Writer
	Err        io.Writer
	Args       map[string]string
	Tag        string
	Dockerfile string
	Context    string
}

// ExecSpec is the input to Host.Exec. Interactive selects `-it` vs `-i`.
type ExecSpec struct {
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	Container   string
	User        string
	Workdir     string
	Argv        []string
	Interactive bool
}

// NewHostClient returns the production Host backed by the moby SDK. It honors
// DOCKER_HOST (via dockerclient.FromEnv) so OrbStack / Docker Desktop /
// Colima just work without any extra glue. BuildImage is the one method
// that still shells out to `docker build` - see the BuildImage comment.
func NewHostClient() (HostClient, error) {
	c, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("docker SDK init: %w", err)
	}
	return &hostClient{c: c}, nil
}

type hostClient struct {
	c *dockerclient.Client
}

// SDKClient exposes the underlying moby client so packages that need direct
// API access (e.g. internal/stack) can share the single instance the host
// uses, instead of opening a second connection.
func (h *hostClient) SDKClient() *dockerclient.Client { return h.c }

func (h *hostClient) DaemonReachable(ctx context.Context) error {
	_, err := h.c.Ping(ctx, dockerclient.PingOptions{})
	return err
}

func (h *hostClient) ImageExists(ctx context.Context, image string) bool {
	_, err := h.c.ImageInspect(ctx, image)
	return err == nil
}

func (h *hostClient) RemoveImage(ctx context.Context, image string) error {
	// Best-effort, mirroring ForceRemoveContainer: a missing image or a
	// transient daemon error shouldn't bubble up to the user.
	_, _ = h.c.ImageRemove(ctx, image, dockerclient.ImageRemoveOptions{Force: true})
	return nil
}

// buildCLIArgs assembles the `docker build` argv for a BuildSpec. Split from
// BuildImage so the flag construction (notably the created-label stamp) is
// unit-testable without a daemon.
func buildCLIArgs(spec BuildSpec, created time.Time) []string {
	args := []string{"build"}
	keys := make([]string, 0, len(spec.Args))
	for k := range spec.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys) // stable arg order for testability and reproducible cache hits
	for _, k := range keys {
		args = append(args, "--build-arg", k+"="+spec.Args[k])
	}
	args = append(args, "--label", ImageCreatedLabel+"="+created.UTC().Format(ImageCreatedLabelFormat))
	args = append(args, "-t", spec.Tag, "-f", spec.Dockerfile, spec.Context)
	return args
}

// BuildImage keeps shelling out to `docker build`. The CLI's build path is
// what gives us BuildKit-by-default and a polished progress UI for free; the
// SDK's ImageBuild returns a raw stream we'd have to parse and re-render to
// match. This is the single intentional shell-out site after the migration.
func (h *hostClient) BuildImage(ctx context.Context, spec BuildSpec) error {
	//nolint:gosec // G204: docker subcommand args are built internally from a validated ContainerSpec, not user shell input.
	cmd := exec.CommandContext(ctx, "docker", buildCLIArgs(spec, time.Now())...)
	cmd.Stdout = orWriter(spec.Out, os.Stdout)
	cmd.Stderr = orWriter(spec.Err, os.Stderr)
	return cmd.Run()
}

func (h *hostClient) ContainerLogs(ctx context.Context, name string, tail int, follow bool, stdout, stderr io.Writer) error {
	opts := dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: follow}
	if tail > 0 {
		opts.Tail = strconv.Itoa(tail)
	}
	rc, err := h.c.ContainerLogs(ctx, name, opts)
	if err != nil {
		return err
	}
	defer rc.Close()
	// Non-TTY containers multiplex stdout+stderr behind 8-byte stdcopy frame
	// headers; StdCopy splits them back out into clean streams.
	_, err = stdcopy.StdCopy(orWriter(stdout, os.Stdout), orWriter(stderr, os.Stderr), rc)
	return err
}

func (h *hostClient) StopContainer(ctx context.Context, name string) error {
	_, err := h.c.ContainerStop(ctx, name, dockerclient.ContainerStopOptions{})
	return err
}

func (h *hostClient) RestartContainer(ctx context.Context, name string) error {
	_, err := h.c.ContainerRestart(ctx, name, dockerclient.ContainerRestartOptions{})
	return err
}

func (h *hostClient) ForceRemoveContainer(ctx context.Context, name string) error {
	// Best-effort cleanup matching the old `docker rm -f` semantics: a missing
	// container is not an error, and a transient daemon hiccup shouldn't bubble
	// up to the user.
	_, _ = h.c.ContainerRemove(ctx, name, dockerclient.ContainerRemoveOptions{Force: true})
	return nil
}

// Exec mirrors `docker exec [-it|-i] -u <user> -w <wd> <container> <argv...>`.
// In TTY mode it puts the local terminal into raw mode, forwards SIGWINCH to
// the remote PTY, and copies the raw byte stream both ways. In non-TTY mode
// it demuxes stdout/stderr via stdcopy. Non-zero exit code is surfaced as an
// *ExitError so callers can propagate it to os.Exit.
func (h *hostClient) Exec(ctx context.Context, spec ExecSpec) error {
	in := orReader(spec.In, os.Stdin)
	out := orWriter(spec.Out, os.Stdout)
	errW := orWriter(spec.Err, os.Stderr)
	tty := spec.Interactive

	createOpts := dockerclient.ExecCreateOptions{
		User:         spec.User,
		WorkingDir:   spec.Workdir,
		Cmd:          spec.Argv,
		TTY:          tty,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}
	if tty {
		// Seed the remote PTY with the current terminal size, if we have one.
		// Without this the daemon falls back to 80x24, which is jarring for any
		// modern terminal window.
		if w, ht, gerr := term.GetSize(int(os.Stdin.Fd())); gerr == nil {
			createOpts.ConsoleSize = dockerclient.ConsoleSize{Width: uint(w), Height: uint(ht)}
		}
	}
	create, err := h.c.ExecCreate(ctx, spec.Container, createOpts)
	if err != nil {
		return err
	}

	attach, err := h.c.ExecAttach(ctx, create.ID, dockerclient.ExecAttachOptions{TTY: tty})
	if err != nil {
		return err
	}
	defer attach.Close()

	// TTY mode: switch the local terminal into raw mode so keystrokes go
	// straight to the container and we never get our own line discipline in
	// the way. Restore on exit.
	if tty && term.IsTerminal(int(os.Stdin.Fd())) {
		st, raerr := term.MakeRaw(int(os.Stdin.Fd()))
		if raerr == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), st) }()
		}
		// SIGWINCH → remote PTY resize.
		resize := make(chan os.Signal, 1)
		signal.Notify(resize, syscall.SIGWINCH)
		defer signal.Stop(resize)
		go func() {
			for range resize {
				w, ht, gerr := term.GetSize(int(os.Stdin.Fd()))
				if gerr == nil {
					_, _ = h.c.ExecResize(ctx, create.ID, dockerclient.ExecResizeOptions{
						Width: uint(w), Height: uint(ht),
					})
				}
			}
		}()
	}

	// In TTY mode the daemon writes the raw terminal byte stream - no framing.
	// In non-TTY mode it multiplexes stdout/stderr into the docker stream
	// protocol, which stdcopy.StdCopy knows how to demux.
	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		if tty {
			_, _ = io.Copy(out, attach.Reader)
		} else {
			_, _ = stdcopy.StdCopy(out, errW, attach.Reader)
		}
	}()

	go func() {
		_, _ = io.Copy(attach.Conn, in)
		_ = attach.CloseWrite()
	}()

	// The output stream EOFs when the exec'd process exits - that's our cue.
	// We deliberately don't wait on the stdin goroutine: it may be blocked in
	// a read of os.Stdin that won't return until the process is torn down by
	// the runtime, and waiting would deadlock interactive shells.
	<-outDone

	insp, err := h.c.ExecInspect(ctx, create.ID, dockerclient.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if insp.ExitCode != 0 {
		return &ExitError{Code: insp.ExitCode}
	}
	return nil
}

// ExitError is returned by Host.Exec when the exec'd command exits non-zero.
// It satisfies errors.As against `interface{ ExitCode() int }`, the same shape
// that *os/exec.ExitError satisfies, so callers can drop a single check and
// propagate the code to os.Exit regardless of which Host method failed.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("docker exec: exit %d", e.Code) }
func (e *ExitError) ExitCode() int { return e.Code }

func orReader(r io.Reader, fallback io.Reader) io.Reader {
	if r == nil {
		return fallback
	}
	return r
}

func orWriter(w io.Writer, fallback io.Writer) io.Writer {
	if w == nil {
		return fallback
	}
	return w
}
