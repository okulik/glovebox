#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"



setup_file() {
  ensure_env
  stack_up
}

test_10-stack-up_stack_reports_infrastructure_services_running() {
  run compose ps --status running --format '{{.Name}}'
  assert_success
  assert_output --partial "glovebox-egress-proxy"
  assert_output --partial "glovebox-stack-controller"
}

TESTS=(
  test_10-stack-up_stack_reports_infrastructure_services_running
)
