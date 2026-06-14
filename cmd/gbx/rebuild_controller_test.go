package main

import "testing"

// The --controller mode targets the singleton stack-controller, so a project
// id or --all is meaningless and must be rejected. The guard fires before any
// docker access, so these run without a daemon.
func TestRebuildControllerRejectsProjectArgs(t *testing.T) {
	if _, _, code := runCLI(t, "rebuild", "--controller", "--all"); code == 0 {
		t.Error("expected non-zero exit for --controller with --all")
	}
	if _, _, code := runCLI(t, "rebuild", "--controller", "someproj"); code == 0 {
		t.Error("expected non-zero exit for --controller with a project id")
	}
}
