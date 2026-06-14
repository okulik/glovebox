#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAB_STATE() { printf '%s/AAB-state' "${GBX_CONFIG_DIR}"; }

setup_file() {
  ensure_env
  install -d "$(_AAB_STATE)"
  local ws1 ws2
  ws1="$(mktemp -d)"; ws2="$(mktemp -d)"
  printf '%s\n' "$ws1" > "$(_AAB_STATE)/ws1"
  printf '%s\n' "$ws2" > "$(_AAB_STATE)/ws2"
  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  "${REPO_ROOT}/bin/gbx" new "$ws2" >/dev/null
  printf '%s\n' "$( _pid_hash "$ws1" )" > "$(_AAB_STATE)/pid1"
  printf '%s\n' "$( _pid_hash "$ws2" )" > "$(_AAB_STATE)/pid2"
}

teardown_file() {
  local sd; sd="$(_AAB_STATE)"
  local pid1="" pid2="" ws1="" ws2=""
  [[ -f "$sd/pid1" ]] && pid1="$(<"$sd/pid1")"
  [[ -f "$sd/pid2" ]] && pid2="$(<"$sd/pid2")"
  [[ -f "$sd/ws1"  ]] && ws1="$(<"$sd/ws1")"
  [[ -f "$sd/ws2"  ]] && ws2="$(<"$sd/ws2")"
  [[ -n "$pid1" ]] && docker rm -f "glovebox-agent-${pid1}" >/dev/null 2>&1 || true
  [[ -n "$pid2" ]] && docker rm -f "glovebox-agent-${pid2}" >/dev/null 2>&1 || true
  [[ -n "$ws1"  ]] && rm -rf "$ws1" || true
  [[ -n "$ws2"  ]] && rm -rf "$ws2" || true
  rm -rf "$sd"
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

test_AAB-project-use_flips_default_to_second_project() {
  local sd; sd="$(_AAB_STATE)"
  local pid2; pid2="$(<"$sd/pid2")"

  run "${REPO_ROOT}/bin/gbx" use "$pid2"
  assert_success
  assert_output --partial "Default project:"

  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid2" "$active_pid"
}

test_AAB-project-use_accepts_prefix() {
  local sd; sd="$(_AAB_STATE)"
  local pid1; pid1="$(<"$sd/pid1")"
  run "${REPO_ROOT}/bin/gbx" use "${pid1:0:6}"
  assert_success
  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid1" "$active_pid"
}

test_AAB-project-use_rejects_unknown_pid() {
  run "${REPO_ROOT}/bin/gbx" use "deadbeefdead"
  assert_failure
  assert_output --partial "No project matches"
}

TESTS=(
  test_AAB-project-use_flips_default_to_second_project
  test_AAB-project-use_accepts_prefix
  test_AAB-project-use_rejects_unknown_pid
)
