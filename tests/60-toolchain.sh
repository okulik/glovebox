#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_60_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_60_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_60_WS_FILE"
}

test_60-toolchain_npm_is_on_path_for_the_gbx_user() {
  run in_agent npm --version
  assert_success
  assert_output --regexp '^[0-9]+\.[0-9]+\.[0-9]+'
}

test_60-toolchain_npm_config_prefix_is_set_to_state_dir() {
  run in_agent bash -lc 'echo "$NPM_CONFIG_PREFIX"'
  assert_success
  assert_output "/home/gbx/.npm"
}

test_60-toolchain_npm_bin_is_on_path() {
  run in_agent bash -lc 'echo "$PATH"'
  assert_success
  assert_output --partial "/home/gbx/.npm/bin"
}

test_60-toolchain_uv_is_on_path() {
  run in_agent uv --version
  assert_success
}

TESTS=(
  test_60-toolchain_npm_is_on_path_for_the_gbx_user
  test_60-toolchain_npm_config_prefix_is_set_to_state_dir
  test_60-toolchain_npm_bin_is_on_path
  test_60-toolchain_uv_is_on_path
)
