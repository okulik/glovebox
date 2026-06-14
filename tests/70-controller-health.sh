#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  stack_up
}

test_70-controller-health_socket_proxy_serves_version_from_control_net() {
  # socket-proxy exposes a restricted Docker API; /version is on the allowlist.
  # It MUST be reachable from glovebox-control (where the controller will live).
  run docker run --rm --network glovebox-control curlimages/curl:8.10.1 \
    -sS -o /dev/null -w "%{http_code}" --max-time 5 \
    http://socket-proxy:2375/version
  assert_success
  assert_output "200"
}

test_70-controller-health_socket_proxy_not_reachable_from_agent_net() {
  # The agent network MUST NOT have a path to socket-proxy. Even with the
  # restricted endpoint allowlist, exposing Docker API endpoints to the agent
  # would defeat the harness's isolation model.
  run docker run --rm --network glovebox-internal curlimages/curl:8.10.1 \
    -sS -o /dev/null -w "%{http_code}" --max-time 3 \
    http://socket-proxy:2375/version
  # Should either fail outright (DNS / connection refused) or never return 200.
  if [[ "$status" -eq 0 && "$output" == "200" ]]; then
    fail "socket-proxy reachable from glovebox-internal; agent could call Docker"
  fi
}

test_70-controller-health_controller_internal_socket() {
  # Agent-callable surface on glovebox-internal.
  run docker run --rm --network glovebox-internal curlimages/curl:8.10.1 \
    -sS --max-time 5 http://stack-controller:7000/health
  assert_success
  assert_output --partial '"status":"ok"'
}

test_70-controller-health_controller_host_socket() {
  # Host-only surface on 127.0.0.1.
  run curl -sS --max-time 5 $(controller_url)/health
  assert_success
  assert_output --partial '"status":"ok"'
}

TESTS=(
  test_70-controller-health_socket_proxy_serves_version_from_control_net
  test_70-controller-health_socket_proxy_not_reachable_from_agent_net
  test_70-controller-health_controller_internal_socket
  test_70-controller-health_controller_host_socket
)
