#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_65_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  # Ensure a project exists before tearing the stack down, so the auto-start
  # test has something to route to.
  create_test_workspace "$_65_WS_FILE" >/dev/null
  # Now tear the stack back down to test auto-start.
  compose stop >/dev/null 2>&1 || true
}

teardown_file() {
  remove_test_workspace "$_65_WS_FILE"
}

test_65-wrapper-autostart_gbx_agent_auto_starts_the_stack_when_it_s_down() {
  # Pre-condition: egress-proxy not running (stack is stopped).
  run compose ps --status running --format '{{.Name}}'
  refute_output --partial "glovebox-egress-proxy"

  run "${REPO_ROOT}/bin/gbx" run claude --help
  assert_success

  # Post-condition: infrastructure is up.
  run compose ps --status running --format '{{.Name}}'
  assert_output --partial "glovebox-egress-proxy"
}

TESTS=(
  test_65-wrapper-autostart_gbx_agent_auto_starts_the_stack_when_it_s_down
)
