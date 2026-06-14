#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_64_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  create_test_workspace "$_64_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_64_WS_FILE"
}

test_64-wrapper-agent-dispatch_gbx_run_claude_help_routes_to_the_claude_binary() {
  run "${REPO_ROOT}/bin/gbx" run claude --help
  assert_success
}

test_64-wrapper-agent-dispatch_bare_agent_name_is_rejected_as_unknown_command() {
  run "${REPO_ROOT}/bin/gbx" claude --help
  assert_failure
  assert_output --partial "Unknown command"
}

test_64-wrapper-agent-dispatch_gbx_unknown_agent_fails_with_helpful_error() {
  run "${REPO_ROOT}/bin/gbx" nosuchagent --help
  assert_failure
  assert_output --partial "Unknown command"
}

test_64-wrapper-agent-dispatch_gbx_help_lists_agents_under_an_agents_heading() {
  run "${REPO_ROOT}/bin/gbx" help
  assert_success
  assert_output --partial "Agents:"
  for a in claude codex opencode pi gemini aider hermes; do
    assert_output --partial "$a"
  done
}

test_64-wrapper-agent-dispatch_gbx_help_groups_stack_and_shell_commands() {
  run "${REPO_ROOT}/bin/gbx" help
  assert_success
  assert_output --partial "stack <subcommand>"
  assert_output --partial "bash shell"
}

TESTS=(
  test_64-wrapper-agent-dispatch_gbx_run_claude_help_routes_to_the_claude_binary
  test_64-wrapper-agent-dispatch_bare_agent_name_is_rejected_as_unknown_command
  test_64-wrapper-agent-dispatch_gbx_unknown_agent_fails_with_helpful_error
  test_64-wrapper-agent-dispatch_gbx_help_lists_agents_under_an_agents_heading
  test_64-wrapper-agent-dispatch_gbx_help_groups_stack_and_shell_commands
)
