#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"



setup_file() {
  ensure_env
  stack_up
}

test_24-no-host-port-bind_egress_proxy_does_not_publish_to_host() {
  # The egress-proxy must not be reachable directly from the host; only agents
  # (on glovebox-internal) can talk to it.
  run compose ps egress-proxy --format '{{.Publishers}}'
  assert_success
  refute_output --partial "host="
}

test_24-no-host-port-bind_socket_proxy_does_not_publish_to_host() {
  # socket-proxy is on glovebox-control only; never host-bound.
  run compose ps socket-proxy --format '{{.Publishers}}'
  assert_success
  refute_output --partial "host="
}

TESTS=(
  test_24-no-host-port-bind_egress_proxy_does_not_publish_to_host
  test_24-no-host-port-bind_socket_proxy_does_not_publish_to_host
)
