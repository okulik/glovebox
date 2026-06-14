#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_50_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_50_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_50_WS_FILE"
}

test_50-smoke-agent-versions_claude_version_reports_a_version_string() {
  run in_agent claude --version
  assert_success
  # Version strings reasonably contain at least one digit.
  [[ "$output" =~ [0-9] ]]
}

test_50-smoke-agent-versions_gemini_version_reports_a_version_string() {
  # Gemini CLI behavior can vary by release; try common version flags and only
  # require that one of them prints something that looks like a semantic version.
  run in_agent bash -lc 'set +e; out="$(gemini --version 2>&1)"; rc=$?; if [[ $rc -ne 0 ]]; then out="$(gemini -v 2>&1)"; rc=$?; fi; printf "%s\n" "$out"; exit 0'
  assert_success
  [[ "$output" =~ [0-9]+\.[0-9]+ ]]
}

TESTS=(
  test_50-smoke-agent-versions_claude_version_reports_a_version_string
  test_50-smoke-agent-versions_gemini_version_reports_a_version_string
)
