#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_66_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_66_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_66_WS_FILE"
}

test_66-agent-auth_agent_auth_lists_all_seven_agents() {
  run in_agent agent-auth
  assert_success
  for a in claude codex opencode pi gemini aider hermes; do
    assert_output --partial "$a"
  done
}

test_66-agent-auth_agent_auth_agent_probes_that_agent_specifically() {
  run in_agent agent-auth gemini
  assert_success
  assert_output --partial "gemini"
}

TESTS=(
  test_66-agent-auth_agent_auth_lists_all_seven_agents
  test_66-agent-auth_agent_auth_agent_probes_that_agent_specifically
)
