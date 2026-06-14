#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_22_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_22_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_22_WS_FILE"
}

test_22-no-new-privs_nonewprivs_flag_is_set_on_the_claude_process() {
  run in_agent bash -c 'grep ^NoNewPrivs /proc/self/status'
  assert_success
  # /proc/self/status uses a literal tab between the key and value.
  # Use partial matching to avoid editor tab-vs-space ambiguity.
  assert_output --partial "NoNewPrivs:"
  assert_output --partial "1"
}

TESTS=(
  test_22-no-new-privs_nonewprivs_flag_is_set_on_the_claude_process
)
