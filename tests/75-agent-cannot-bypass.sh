#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_75_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_75_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_75_WS_FILE"
}

test_75_no_docker_socket_in_agent() {
  run in_agent test -e /var/run/docker.sock
  assert_failure
}

test_75_docker_cli_absent_or_no_perm() {
  # If docker exists in the image, it should fail to connect (no socket).
  run in_agent bash -c 'command -v docker >/dev/null 2>&1 && echo present || echo missing'
  assert_success
  if [[ "$output" == "present" ]]; then
    run in_agent docker ps
    assert_failure
  fi
}

test_75_socket_proxy_unreachable_from_agent() {
  # From the agent's network, socket-proxy:2375 must not resolve / connect.
  run in_agent bash -c 'curl -sS -o /dev/null -w "%{http_code}" --max-time 3 http://socket-proxy:2375/version 2>/dev/null || echo BLOCKED'
  assert_success
  # Either the curl errors out (output may include BLOCKED), or returns non-200, but NOT 200.
  refute_output "200"
}

TESTS=(
  test_75_no_docker_socket_in_agent
  test_75_docker_cli_absent_or_no_perm
  test_75_socket_proxy_unreachable_from_agent
)
