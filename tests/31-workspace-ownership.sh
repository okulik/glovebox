#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_31_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_31_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_31_WS_FILE"
}

test_31-workspace-ownership_file_written_inside_container_appears_as_host_uid_gid_on_the_host() {
  local ws fname
  ws="$(<"$_31_WS_FILE")"
  fname="harness-owner-test-$$"
  in_agent bash -c "echo hi > /workspace/${fname}"

  # macOS stat: -f. Linux stat: -c. Detect.
  if [[ "$(uname)" == "Darwin" ]]; then
    run stat -f '%u:%g' "${ws}/${fname}"
  else
    run stat -c '%u:%g' "${ws}/${fname}"
  fi
  assert_success
  assert_output "501:20"

  rm -f "${ws}/${fname}"
}

TESTS=(
  test_31-workspace-ownership_file_written_inside_container_appears_as_host_uid_gid_on_the_host
)
