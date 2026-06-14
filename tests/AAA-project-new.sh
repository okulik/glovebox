#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
}

teardown_file() {
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

setup() {
  # Each test starts with no default project so "first invocation sets default"
  # and "second invocation does not overwrite" are independently verifiable.
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

test_AAA-project-new_first_invocation_creates_agent_and_sets_default() {
  local ws pid
  ws="$(mktemp -d)"

  run "${REPO_ROOT}/bin/gbx" new "$ws"
  assert_success
  assert_output --partial "Set as default."

  pid="$( _pid_hash "$ws" )"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"

  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid" "$active_pid"

  docker rm -f "glovebox-agent-${pid}" >/dev/null
  rm -rf "$ws"
}

test_AAA-project-new_second_path_does_not_overwrite_default() {
  local ws1 ws2 pid1 pid2
  ws1="$(mktemp -d)"; ws2="$(mktemp -d)"

  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  pid1="$( _pid_hash "$ws1" )"

  run "${REPO_ROOT}/bin/gbx" new "$ws2"
  assert_success
  assert_output --partial "default remains"

  pid2="$( _pid_hash "$ws2" )"
  local active_pid
  active_pid="$(head -n1 "${GBX_CONFIG_DIR}/active-project")"
  assert_equal "$pid1" "$active_pid"

  docker rm -f "glovebox-agent-${pid1}" "glovebox-agent-${pid2}" >/dev/null
  rm -rf "$ws1" "$ws2"
}

test_AAA-project-new_re_registering_same_path_is_idempotent() {
  local ws pid
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  pid="$( _pid_hash "$ws" )"
  local cid_before
  cid_before="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"

  run "${REPO_ROOT}/bin/gbx" new "$ws"
  assert_success
  assert_output --partial "Already registered as ${pid}"

  local cid_after
  cid_after="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"
  assert_equal "$cid_before" "$cid_after"

  docker rm -f "glovebox-agent-${pid}" >/dev/null
  rm -rf "$ws"
}

test_AAA-project-new_rejects_missing_directory() {
  run "${REPO_ROOT}/bin/gbx" new "/this/path/should/not/exist/$(date +%s%N)"
  assert_failure
}

TESTS=(
  test_AAA-project-new_first_invocation_creates_agent_and_sets_default
  test_AAA-project-new_second_path_does_not_overwrite_default
  test_AAA-project-new_re_registering_same_path_is_idempotent
  test_AAA-project-new_rejects_missing_directory
)
