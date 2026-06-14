#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_89_state_dir() { printf '%s/89-state' "${GBX_CONFIG_DIR}"; }

setup_file() {
  ensure_env
  local sd; sd="$(_89_state_dir)"
  install -d "$sd"
  local ws1 ws2
  ws1="$(mktemp -d)"; ws2="$(mktemp -d)"
  printf '%s\n' "$ws1" > "${sd}/ws1"
  printf '%s\n' "$ws2" > "${sd}/ws2"
  local pid1 pid2
  pid1="$( _pid_hash "$ws1" )"
  pid2="$( _pid_hash "$ws2" )"
  printf '%s\n' "$pid1" > "${sd}/pid1"
  printf '%s\n' "$pid2" > "${sd}/pid2"
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

teardown_file() {
  local sd; sd="$(_89_state_dir)"
  local pid1="" pid2="" ws1="" ws2=""
  [[ -f "${sd}/pid1" ]] && pid1="$(<"${sd}/pid1")"
  [[ -f "${sd}/pid2" ]] && pid2="$(<"${sd}/pid2")"
  [[ -f "${sd}/ws1"  ]] && ws1="$(<"${sd}/ws1")"
  [[ -f "${sd}/ws2"  ]] && ws2="$(<"${sd}/ws2")"
  [[ -n "$pid1" ]] && docker rm -f "glovebox-agent-${pid1}" >/dev/null 2>&1 || true
  [[ -n "$pid2" ]] && docker rm -f "glovebox-agent-${pid2}" >/dev/null 2>&1 || true
  [[ -n "$ws1"  ]] && rm -rf "$ws1" || true
  [[ -n "$ws2"  ]] && rm -rf "$ws2" || true
  rm -rf "$sd"
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

test_89-project-new_first_run_creates_agent_and_sets_default() {
  local sd; sd="$(_89_state_dir)"
  local ws1; ws1="$(<"${sd}/ws1")"
  local pid1; pid1="$(<"${sd}/pid1")"
  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid1}")"
  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid1" "$active_pid"
}

test_89-project-new_second_path_creates_second_agent_first_keeps_running() {
  local sd; sd="$(_89_state_dir)"
  local ws2; ws2="$(<"${sd}/ws2")"
  local pid1; pid1="$(<"${sd}/pid1")"
  local pid2; pid2="$(<"${sd}/pid2")"
  "${REPO_ROOT}/bin/gbx" new "$ws2" >/dev/null
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid2}")"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid1}")"
  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid1" "$active_pid"
}

test_89-project-use_flips_default_pointer() {
  local sd; sd="$(_89_state_dir)"
  local pid2; pid2="$(<"${sd}/pid2")"
  "${REPO_ROOT}/bin/gbx" use "$pid2" >/dev/null
  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid2" "$active_pid"
}

TESTS=(
  test_89-project-new_first_run_creates_agent_and_sets_default
  test_89-project-new_second_path_creates_second_agent_first_keeps_running
  test_89-project-use_flips_default_pointer
)
