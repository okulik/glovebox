#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAG_WS="$(mktemp)"

setup_file() {
  ensure_env
  local ws; ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_AAG_WS"
  # Project exists; remove the default pointer to reach "no default" state.
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

teardown_file() {
  local ws=""
  [[ -f "$_AAG_WS" ]] && ws="$(<"$_AAG_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_AAG_WS" "${GBX_CONFIG_DIR}/active-project"
}

test_AAG_run_refuses_with_no_default_project() {
  run "${REPO_ROOT}/bin/gbx" run -- true
  assert_failure
  assert_output --partial "No default project."
}

test_AAG_run_with_p_works() {
  local ws; ws="$(<"$_AAG_WS")"
  local pid
  pid="$( _pid_hash "$ws" )"
  run "${REPO_ROOT}/bin/gbx" -p "$pid" run -- echo "ok"
  assert_success
  # --partial because docker exec may emit incidental noise (e.g. TTY warnings)
  # alongside the echo output under some daemon/host combinations.
  assert_output --partial "ok"
}

TESTS=(
  test_AAG_run_refuses_with_no_default_project
  test_AAG_run_with_p_works
)
