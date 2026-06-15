package dockerx

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestFakeHostRecordsCalls(t *testing.T) {
	f := NewFakeHost()
	ctx := context.Background()
	_ = f.DaemonReachable(ctx)
	_ = f.StopContainer(ctx, "glovebox-agent-aaa")
	_ = f.RestartContainer(ctx, "glovebox-agent-aaa")
	_ = f.ForceRemoveContainer(ctx, "glovebox-agent-aaa")

	want := []string{
		"daemon-reachable",
		"stop glovebox-agent-aaa",
		"restart glovebox-agent-aaa",
		"rm -f glovebox-agent-aaa",
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Errorf("calls = %v, want %v", f.Calls, want)
	}
}

func TestFakeHostImageExistsRespectsImagesMap(t *testing.T) {
	f := NewFakeHost()
	if f.ImageExists(context.Background(), "glovebox-agent:local") {
		t.Error("unseeded image should not exist")
	}
	f.Images["glovebox-agent:local"] = true
	if !f.ImageExists(context.Background(), "glovebox-agent:local") {
		t.Error("seeded image should exist")
	}
}

func TestFakeHostBuildMarksImagePresent(t *testing.T) {
	f := NewFakeHost()
	if err := f.BuildImage(context.Background(), BuildSpec{Tag: "glovebox-agent:local"}); err != nil {
		t.Fatalf("BuildImage: %v", err)
	}
	if !f.ImageExists(context.Background(), "glovebox-agent:local") {
		t.Error("BuildImage should make the tag exist on subsequent ImageExists checks")
	}
	if f.LastBuild.Tag != "glovebox-agent:local" {
		t.Errorf("LastBuild.Tag = %q", f.LastBuild.Tag)
	}
}

func TestFakeHostRemoveImageClearsPresence(t *testing.T) {
	f := NewFakeHost()
	if err := f.BuildImage(context.Background(), BuildSpec{Tag: "glovebox-agent-abc:local"}); err != nil {
		t.Fatalf("BuildImage: %v", err)
	}
	if !f.ImageExists(context.Background(), "glovebox-agent-abc:local") {
		t.Fatal("image should exist after build")
	}
	if err := f.RemoveImage(context.Background(), "glovebox-agent-abc:local"); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}
	if f.ImageExists(context.Background(), "glovebox-agent-abc:local") {
		t.Error("image should be gone after RemoveImage")
	}
}

func TestFakeHostExecCapturesSpec(t *testing.T) {
	f := NewFakeHost()
	spec := ExecSpec{
		Container:   "glovebox-agent-xyz",
		Interactive: true,
		User:        "501:20",
		Workdir:     "/workspace",
		Argv:        []string{"bash", "-l"},
	}
	if err := f.Exec(context.Background(), spec); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !reflect.DeepEqual(f.LastExec.Argv, []string{"bash", "-l"}) {
		t.Errorf("LastExec.Argv = %v", f.LastExec.Argv)
	}
	if !strings.HasPrefix(f.Calls[0], "exec glovebox-agent-xyz") {
		t.Errorf("call entry = %q", f.Calls[0])
	}
}
