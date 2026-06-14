#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_84_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_84_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_84_WS_FILE"
}

test_84_gbx_stack_is_on_path() {
  run in_agent which gbx-stack
  assert_success
}

test_84_status_works_when_no_stack() {
  run in_agent gbx-stack status
  assert_success
  assert_output --partial "state"
}

test_84_start_requires_service_arg() {
  run in_agent gbx-stack start
  assert_failure
}

test_84_propose_submits_to_controller() {
  local tmp=/tmp/proposed-test.yml
  in_agent sh -c "cat > $tmp <<EOF
version: 1
services:
  redis:
    image: redis:7-alpine
EOF
"
  run in_agent gbx-stack propose $tmp
  assert_success
  assert_output --partial '"status":"proposed"'
  in_agent rm -f $tmp || true
}

TESTS=(
  test_84_gbx_stack_is_on_path
  test_84_status_works_when_no_stack
  test_84_start_requires_service_arg
  test_84_propose_submits_to_controller
)
