package dockerx

import (
	"slices"
	"testing"
	"time"
)

func TestBuildCLIArgs_StampsImageCreatedLabel(t *testing.T) {
	created := time.Date(2024, 9, 9, 14, 16, 46, 786_000_000, time.UTC)
	args := buildCLIArgs(BuildSpec{
		Tag:        "glovebox-agent:local",
		Dockerfile: "docker/Dockerfile",
		Context:    ".",
	}, created)

	i := slices.Index(args, "--label")
	if i == -1 || i+1 >= len(args) {
		t.Fatalf("no --label flag in args: %v", args)
	}
	want := "io.glovebox.image.created=2024-09-09T14:16:46.786Z"
	if args[i+1] != want {
		t.Errorf("label = %q, want %q", args[i+1], want)
	}
}

func TestBuildCLIArgs_LabelTimeIsUTC(t *testing.T) {
	// A non-UTC build time must still render as a Z-suffixed UTC timestamp.
	loc := time.FixedZone("CEST", 2*60*60)
	created := time.Date(2024, 9, 9, 16, 16, 46, 786_000_000, loc)
	args := buildCLIArgs(BuildSpec{Tag: "t", Dockerfile: "f", Context: "."}, created)

	i := slices.Index(args, "--label")
	if i == -1 {
		t.Fatalf("no --label flag in args: %v", args)
	}
	want := "io.glovebox.image.created=2024-09-09T14:16:46.786Z"
	if args[i+1] != want {
		t.Errorf("label = %q, want %q", args[i+1], want)
	}
}
