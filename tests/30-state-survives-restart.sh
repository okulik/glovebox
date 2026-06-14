#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_30_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_30_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_30_WS_FILE"
}

test_30-state-survives-restart_sentinel_file_in_claude_survives_stop_start() {
  local sentinel="harness-test-$(date +%s)-$RANDOM"
  local c
  c="$(_active_agent_container)"

  in_agent bash -c "echo persist > ~/.claude/${sentinel}"
  run in_agent cat "/home/gbx/.claude/${sentinel}"
  assert_success
  assert_output --partial "persist"

  docker stop "$c" >/dev/null
  docker start "$c" >/dev/null
  # Wait for the container to be running again.
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if docker exec "$c" true 2>/dev/null; then break; fi
    sleep 1
  done

  run in_agent cat "/home/gbx/.claude/${sentinel}"
  assert_success
  assert_output --partial "persist"

  in_agent rm -f "/home/gbx/.claude/${sentinel}"
}

TESTS=(
  test_30-state-survives-restart_sentinel_file_in_claude_survives_stop_start
)
