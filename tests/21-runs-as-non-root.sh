#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_21_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_21_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_21_WS_FILE"
}

test_21-runs-as-non-root_claude_user_is_uid_501_gid_20() {
  run in_agent id
  assert_success
  assert_output --partial "uid=501"
  assert_output --partial "gid=20"
}

test_21-runs-as-non-root_default_shell_pwd_is_workspace() {
  run in_agent pwd
  assert_success
  assert_output "/workspace"
}

TESTS=(
  test_21-runs-as-non-root_claude_user_is_uid_501_gid_20
  test_21-runs-as-non-root_default_shell_pwd_is_workspace
)
