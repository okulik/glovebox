#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  WS1="$(mktemp -d)"; WS2="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$WS1" >/dev/null
  "${REPO_ROOT}/bin/gbx" new "$WS2" >/dev/null   # active = WS1 (first registered)
  PID1="$( _pid_hash "$WS1" )"
  PID2="$( _pid_hash "$WS2" )"
  install -d "${GBX_CONFIG_DIR}/91-state"
  printf '%s\n' "$WS1" > "${GBX_CONFIG_DIR}/91-state/ws1"
  printf '%s\n' "$PID1" > "${GBX_CONFIG_DIR}/91-state/pid1"
  printf '%s\n' "$PID2" > "${GBX_CONFIG_DIR}/91-state/pid2"
}

teardown_file() {
  local pid1 pid2
  pid1="$(cat "${GBX_CONFIG_DIR}/91-state/pid1" 2>/dev/null || true)"
  pid2="$(cat "${GBX_CONFIG_DIR}/91-state/pid2" 2>/dev/null || true)"
  [[ -n "$pid1" ]] && docker rm -f "glovebox-agent-${pid1}" >/dev/null 2>&1 || true
  [[ -n "$pid2" ]] && docker rm -f "glovebox-agent-${pid2}" >/dev/null 2>&1 || true
  rm -rf "${GBX_CONFIG_DIR}/91-state"
}

test_91-p-flag_id_routes_to_that_project() {
  local pid1
  pid1="$(cat "${GBX_CONFIG_DIR}/91-state/pid1")"
  run "${REPO_ROOT}/bin/gbx" -p "$pid1" run -- hostname
  assert_success
  assert_output --partial "glovebox-${pid1}"
}

test_91-p-flag_does_not_update_active() {
  local before pid1
  pid1="$(cat "${GBX_CONFIG_DIR}/91-state/pid1")"
  before="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  "${REPO_ROOT}/bin/gbx" -p "$pid1" run -- true >/dev/null
  local after
  after="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$before" "$after"
}

test_91-p-flag_unknown_id_errors() {
  run "${REPO_ROOT}/bin/gbx" -p "deadbeefdead" run -- true
  assert_failure
}

TESTS=(
  test_91-p-flag_id_routes_to_that_project
  test_91-p-flag_does_not_update_active
  test_91-p-flag_unknown_id_errors
)
