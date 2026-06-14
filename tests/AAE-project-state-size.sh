#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAE_WS="$(mktemp)"

setup_file() {
  ensure_env
  local ws; ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_AAE_WS"
}

teardown_file() {
  local ws=""
  [[ -f "$_AAE_WS" ]] && ws="$(<"$_AAE_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_AAE_WS" "${GBX_CONFIG_DIR}/active-project"
}

test_AAE-project-state-size_prints_project_and_shared_sections() {
  run "${REPO_ROOT}/bin/gbx" state-size
  assert_success
  assert_output --partial "PROJECT"
  assert_output --partial "SHARED"
}

TESTS=(
  test_AAE-project-state-size_prints_project_and_shared_sections
)
