#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_61_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_61_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_61_WS_FILE"
}

test_61-agents-present_claude_binary_is_present_and_runnable() {
  run in_agent claude --help
  assert_success
}

test_61-agents-present_claude_installs_into_npm_prefix() {
  # After first run, the real binary lives under NPM_CONFIG_PREFIX/bin.
  run in_agent bash -lc 'readlink -f "$(command -v claude)"'
  assert_success
  assert_output --partial "/home/gbx/.npm"
}

test_61-agents-present_codex_binary_is_present_and_runnable() {
  run in_agent codex --help
  assert_success
}

test_61-agents-present_opencode_binary_is_present_and_runnable() {
  run in_agent opencode --help
  assert_success
}

test_61-agents-present_pi_binary_is_present_and_runnable() {
  run in_agent pi --help
  assert_success
}

test_61-agents-present_gemini_binary_is_present_and_runnable() {
  run in_agent gemini --help
  assert_success
}

test_61-agents-present_aider_binary_is_present_and_runnable() {
  run in_agent aider --help
  assert_success
}

test_61-agents-present_hermes_binary_is_present_and_runnable() {
  run in_agent hermes --help
  assert_success
}

TESTS=(
  test_61-agents-present_claude_binary_is_present_and_runnable
  test_61-agents-present_claude_installs_into_npm_prefix
  test_61-agents-present_codex_binary_is_present_and_runnable
  test_61-agents-present_opencode_binary_is_present_and_runnable
  test_61-agents-present_pi_binary_is_present_and_runnable
  test_61-agents-present_gemini_binary_is_present_and_runnable
  test_61-agents-present_aider_binary_is_present_and_runnable
  test_61-agents-present_hermes_binary_is_present_and_runnable
)
