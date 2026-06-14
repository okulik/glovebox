#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_23_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_23_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_23_WS_FILE"
}

test_23-caps-dropped_capability_set_is_empty_cap_drop_all_effective() {
  # capsh comes from libcap2-bin which the Dockerfile installs; it lives in /sbin.
  run in_agent bash -c '/sbin/capsh --print | grep ^Current:'
  assert_success
  # Empty capability set is rendered as "Current: =" by capsh.
  assert_output "Current: ="
}

TESTS=(
  test_23-caps-dropped_capability_set_is_empty_cap_drop_all_effective
)
