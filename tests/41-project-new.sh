#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

teardown_file() {
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

test_41-project-new_updates_pointer_and_mounts_workspace() {
  local target abs pid
  target="$(mktemp -d)"
  abs="$(cd "$target" && pwd -P)"
  echo "marker" > "${target}/marker.txt"

  run "${REPO_ROOT}/bin/gbx" new "$target"
  assert_success

  pid="$( _pid_hash "$abs" )"
  assert_file_contains "${GBX_CONFIG_DIR}/active-project" "$pid"
  assert_file_contains "${GBX_CONFIG_DIR}/active-project" "$abs"

  run docker exec "glovebox-agent-${pid}" cat /workspace/marker.txt
  assert_output --partial "marker"

  docker rm -f "glovebox-agent-${pid}" >/dev/null
  rm -rf "$target"
}

TESTS=(
  test_41-project-new_updates_pointer_and_mounts_workspace
)
