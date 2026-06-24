package agent_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/okulik/glovebox/internal/agent"
)

func TestParseMountSpecBareHost(t *testing.T) {
	host := t.TempDir()
	m, err := agent.ParseMountSpec(host)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantHost, _ := filepath.EvalSymlinks(host)
	if m.Host != wantHost {
		t.Fatalf("host: want %q got %q", wantHost, m.Host)
	}
	if m.Container != "/mnt/"+filepath.Base(wantHost) {
		t.Fatalf("container default: got %q", m.Container)
	}
	if m.Mode != "rw" {
		t.Fatalf("mode default: got %q", m.Mode)
	}
}

func TestParseMountSpecHostAndContainer(t *testing.T) {
	host := t.TempDir()
	m, err := agent.ParseMountSpec(host + ":/data")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Container != "/data" {
		t.Fatalf("container: got %q", m.Container)
	}
	if m.Mode != "rw" {
		t.Fatalf("mode default: got %q", m.Mode)
	}
}

func TestParseMountSpecReadOnly(t *testing.T) {
	host := t.TempDir()
	m, err := agent.ParseMountSpec(host + ":/data:ro")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Mode != "ro" {
		t.Fatalf("mode: got %q", m.Mode)
	}
}

func TestParseMountSpecRejectsBadMode(t *testing.T) {
	host := t.TempDir()
	if _, err := agent.ParseMountSpec(host + ":/data:rwx"); err == nil {
		t.Fatal("want error for invalid mode")
	}
}

func TestParseMountSpecRejectsRelativeContainer(t *testing.T) {
	host := t.TempDir()
	if _, err := agent.ParseMountSpec(host + ":data"); err == nil {
		t.Fatal("want error for relative container path")
	}
}

func TestParseMountSpecRejectsMissingHost(t *testing.T) {
	if _, err := agent.ParseMountSpec("/no/such/path/zz:/data"); err == nil {
		t.Fatal("want error for missing host")
	}
}

func TestParseMountSpecRejectsReservedContainer(t *testing.T) {
	host := t.TempDir()
	for _, c := range []string{"/workspace", "/home/gbx/.claude", "/home/gbx/.npm"} {
		if _, err := agent.ParseMountSpec(host + ":" + c); err == nil {
			t.Errorf("want error for reserved container %q", c)
		}
	}
}

func TestParseMountSpecRejectsEmpty(t *testing.T) {
	if _, err := agent.ParseMountSpec(""); err == nil {
		t.Fatal("want error for empty spec")
	}
}

func TestParseMountSpecRejectsTooManyColons(t *testing.T) {
	host := t.TempDir()
	if _, err := agent.ParseMountSpec(host + ":/a:rw:extra"); err == nil {
		t.Fatal("want error for 4-part spec")
	}
}

func TestMountString(t *testing.T) {
	m := agent.Mount{Host: "/h", Container: "/c", Mode: "ro"}
	if m.String() != "/h:/c:ro" {
		t.Fatalf("String(): got %q", m.String())
	}
}

func TestReadMountsMissingFile(t *testing.T) {
	dir := t.TempDir()
	got, err := agent.ReadMounts(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestReadMountsSkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	body := "\n# a comment\n/h1:/c1:rw\n\n/h2:/c2:ro\n"
	if err := os.WriteFile(filepath.Join(dir, "mounts.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := agent.ReadMounts(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []agent.Mount{
		{Host: "/h1", Container: "/c1", Mode: "rw"},
		{Host: "/h2", Container: "/c2", Mode: "ro"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestReadMountsRejectsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mounts.txt"), []byte("/h:/c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.ReadMounts(dir); err == nil {
		t.Fatal("want error for missing mode")
	}
}

func TestWriteMountsAtomicAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	in := []agent.Mount{
		{Host: "/h1", Container: "/c1", Mode: "rw"},
		{Host: "/h2", Container: "/c2", Mode: "ro"},
	}
	if err := agent.WriteMounts(dir, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "mounts.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/h1:/c1:rw\n") || !strings.Contains(string(data), "/h2:/c2:ro\n") {
		t.Fatalf("on-disk format mismatch: %q", string(data))
	}
	got, err := agent.ReadMounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("roundtrip got %v want %v", got, in)
	}
}

func TestWriteMountsEmptyReplacesFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "mounts.txt"), []byte("/h1:/c1:rw\n"), 0o644)
	if err := agent.WriteMounts(dir, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := agent.ReadMounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("want nil after empty write, got %v", got)
	}
}
