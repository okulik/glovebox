#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAH_WS="$(mktemp)"

setup_file() {
  ensure_env
  local ws
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_AAH_WS"
}

teardown_file() {
  local ws=""
  [[ -f "$_AAH_WS" ]] && ws="$(<"$_AAH_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_AAH_WS" "${GBX_CONFIG_DIR}/active-project"
}

test_AAH-p-flag_rejects_path_argument() {
  local ws; ws="$(<"$_AAH_WS")"
  run "${REPO_ROOT}/bin/gbx" -p "$ws" run -- true
  assert_failure
  assert_output --partial "No project matches"
}

TESTS=(
  test_AAH-p-flag_rejects_path_argument
)
