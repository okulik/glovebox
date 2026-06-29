# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.2.2 - 2026-06-29

Maintenance release. This is an internal refactor, build-tooling, and docs
pass — there are no new features and no user-facing behavior or CLI changes.

### Changed
- Centralized all `GBX_*` environment-variable names as constants in
  `internal/config/env.go`, so producers (the host CLI, stack injection) and
  consumers read the same keys and can't silently diverge.
- Simplified default Docker host name/port handling in the agent runtime.
- Refactored the `agent` package and `BuildCreateConfig`, and adopted shared
  constants across the codebase.
- Merged `internal/projectid` into `internal/project`.
- Slimmed the `Makefile` by extracting its logic into standalone scripts under
  `scripts/` (`release.sh`, `clean-tests.sh`, `uninstall.sh`).

### Removed
- Dropped the now-unused `internal/fsx` and `internal/pathx` packages; their
  remaining helpers were inlined into `internal/agent`.

### Fixed
- Restored the README hero image (`docs/glovebox.jpg`).
