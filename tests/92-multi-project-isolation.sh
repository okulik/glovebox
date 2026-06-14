#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_92_WS1_FILE="$(mktemp)"
_92_WS2_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up >/dev/null
  local ws1 ws2
  ws1="$(mktemp -d)"; ws2="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  "${REPO_ROOT}/bin/gbx" new "$ws2" >/dev/null
  printf '%s' "$ws1" > "$_92_WS1_FILE"
  printf '%s' "$ws2" > "$_92_WS2_FILE"
}

teardown_file() {
  local ws1="" ws2=""
  [[ -f "$_92_WS1_FILE" ]] && ws1="$(<"$_92_WS1_FILE")"
  [[ -f "$_92_WS2_FILE" ]] && ws2="$(<"$_92_WS2_FILE")"
  if [[ -n "$ws1" ]]; then
    docker rm -f "glovebox-agent-$(_pid_hash "$ws1")" >/dev/null 2>&1 || true
    rm -rf "$ws1"
  fi
  if [[ -n "$ws2" ]]; then
    docker rm -f "glovebox-agent-$(_pid_hash "$ws2")" >/dev/null 2>&1 || true
    rm -rf "$ws2"
  fi
  rm -f "$_92_WS1_FILE" "$_92_WS2_FILE" "${GBX_CONFIG_DIR}/active-project"
}

test_92-multi-project_two_agents_run_concurrently() {
  local pid1 pid2
  pid1="$(_pid_hash "$(<"$_92_WS1_FILE")")"
  pid2="$(_pid_hash "$(<"$_92_WS2_FILE")")"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid1}")"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid2}")"
}

test_92-multi-project_state_dirs_are_isolated() {
  local pid1 pid2
  pid1="$(_pid_hash "$(<"$_92_WS1_FILE")")"
  pid2="$(_pid_hash "$(<"$_92_WS2_FILE")")"
  # Drop a marker file inside one agent; the other must not see it.
  docker exec "glovebox-agent-${pid1}" sh -c 'echo project-A > $HOME/.claude/marker'
  run docker exec "glovebox-agent-${pid2}" sh -c 'cat $HOME/.claude/marker 2>/dev/null'
  assert_failure
  run docker exec "glovebox-agent-${pid1}" sh -c 'cat $HOME/.claude/marker'
  assert_output --partial "project-A"
}

test_92-multi-project_shared_cache_is_visible_to_both() {
  local pid1 pid2
  pid1="$(_pid_hash "$(<"$_92_WS1_FILE")")"
  pid2="$(_pid_hash "$(<"$_92_WS2_FILE")")"
  docker exec "glovebox-agent-${pid1}" sh -c 'echo shared > $HOME/.npm/marker'
  run docker exec "glovebox-agent-${pid2}" sh -c 'cat $HOME/.npm/marker'
  assert_output --partial "shared"
}

TESTS=(
  test_92-multi-project_two_agents_run_concurrently
  test_92-multi-project_state_dirs_are_isolated
  test_92-multi-project_shared_cache_is_visible_to_both
)
