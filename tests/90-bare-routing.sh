#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  WS="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$WS" >/dev/null
  local pid
  pid="$( _pid_hash "$WS" )"
  "${REPO_ROOT}/bin/gbx" use "$pid" >/dev/null
  install -d "${GBX_CONFIG_DIR}/90-state"
  cp "${GBX_CONFIG_DIR}/active-project" "${GBX_CONFIG_DIR}/90-state/active-project"
}

teardown_file() {
  local pid
  pid="$(head -n1 "${GBX_CONFIG_DIR}/90-state/active-project" 2>/dev/null || true)"
  [[ -n "$pid" ]] && docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
  rm -rf "${GBX_CONFIG_DIR}/90-state"
}

test_90-bare-routing_bare_gbx_run_executes_in_active_agent() {
  local pid
  pid="$(head -n1 "${GBX_CONFIG_DIR}/90-state/active-project")"
  # Make sure active-project still points at the right project (other tests may have run cd).
  cp "${GBX_CONFIG_DIR}/90-state/active-project" "${GBX_CONFIG_DIR}/active-project"
  run "${REPO_ROOT}/bin/gbx" run -- hostname
  assert_success
  assert_output --partial "glovebox-${pid}"
}

test_90-bare-routing_no_active_project_errors_helpfully() {
  # Snapshot the file, then delete it, then run.
  local backup
  backup="$(mktemp)"
  cp "${GBX_CONFIG_DIR}/active-project" "$backup"
  rm -f "${GBX_CONFIG_DIR}/active-project"

  run "${REPO_ROOT}/bin/gbx" run -- hostname
  assert_failure
  assert_output --partial "No default project"

  # Restore.
  cp "$backup" "${GBX_CONFIG_DIR}/active-project"
  rm -f "$backup"
}

TESTS=(
  test_90-bare-routing_bare_gbx_run_executes_in_active_agent
  test_90-bare-routing_no_active_project_errors_helpfully
)
