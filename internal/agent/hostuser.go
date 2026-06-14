package agent

import (
	"fmt"
	"os"
)

// HostUID and HostGID report the invoking host user's numeric uid/gid. The
// agent container is created and exec'd as this user (see HostUser), and the
// agent image is built with these as HOST_UID/HOST_GID build args, so the
// container's `gbx` user matches the host user 1:1.
//
// This is what keeps bind-mounted files (the workspace, per-project state)
// owned by the invoking user. On macOS the Docker Desktop / OrbStack file-
// sharing layer maps ownership regardless of the container uid, but on native
// Linux a bind mount is direct host filesystem access: the container uid must
// equal the host uid or the agent writes files the host user can't own. Both
// platforms are served by deriving the values at runtime rather than pinning a
// literal (which was the macOS-only 501:20).
func HostUID() int { return os.Getuid() }
func HostGID() int { return os.Getgid() }

// HostUser formats the host uid:gid as a Docker `User` string for container
// create and exec.
func HostUser() string { return fmt.Sprintf("%d:%d", HostUID(), HostGID()) }
