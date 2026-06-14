#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_43_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  create_test_workspace "$_43_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_43_WS_FILE"
}

setup() {
  # Ensure the active agent is running before each test.
  local c
  c="$(_active_agent_container 2>/dev/null)" || return 0
  if [[ -n "$c" ]] && ! docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null | grep -q true; then
    docker start "$c" >/dev/null 2>&1 || true
  fi
}

test_43-wrapper-run_gbx_run_echo_prints_to_stdout() {
  run "${REPO_ROOT}/bin/gbx" run -- echo "hello from inside"
  assert_success
  assert_output "hello from inside"
}

test_43-wrapper-run_gbx_run_exits_with_command_exit_code() {
  run "${REPO_ROOT}/bin/gbx" run -- bash -c "exit 7"
  [[ "$status" -eq 7 ]]
}

TESTS=(
  test_43-wrapper-run_gbx_run_echo_prints_to_stdout
  test_43-wrapper-run_gbx_run_exits_with_command_exit_code
)
