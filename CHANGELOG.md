# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.2.3 - 2026-07-01

Adds a way to view sandboxed agent conversations in desktop viewers, plus some
internal consolidation.

### Added
- `gbx export-conversations`: copies the agent conversation logs written *inside*
  the sandbox out to the host locations desktop viewers (AgentsView and similar)
  scan. Every sandbox runs with `cwd=/workspace`, so the command rewrites each
  session's recorded working directory to a synthetic, glovebox-tagged path
  (`.../gbx-<pid>-<name>`); the viewer then shows it as a distinct project
  `gbx_<pid>_<name>` instead of collapsing every project into one `workspace`.
  Supports `--all` (every project), `--harness <name>`, and `--dest`. Claude Code
  is fully supported; the other bundled harnesses are recognized but scaffolded.
  Re-running is idempotent and drops an earlier export of the same project (e.g.
  from a changed naming scheme) so a session-id-deduping viewer never keeps a
  stale copy.

### Changed
- Centralized the supported agent/harness name set as `agent.Names` in
  `internal/agent`, referenced by the per-project state subdirs, the export
  registry, the gbxa dispatch table, and help text; tests assert the name-keyed
  maps can't drift.
- Centralized container-side filesystem paths (`/workspace`, the `/home/gbx/*`
  agent state dirs, the sandbox docs) in `internal/agent/paths.go`, so the
  container bind targets and the mount guard's reserved-path set derive from one
  source and stay in sync.
- Removed migration- and editor-style notes from code comments.

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
