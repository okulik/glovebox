#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test79"

setup_file() {
  ensure_env
  stack_up
}

teardown_file() {
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
}

test_79_rejected_apply_does_not_become_ready() {
  # Submit a bad manifest (unknown registry). The controller must not bring up
  # any containers, so /status must not report state:"ready".
  curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  x:\n    image: sketchy.example.com/foo:1.0\n' \
    "$(controller_url)/projects/${PID}/apply" >/dev/null

  run curl -sS "$(controller_url)/projects/${PID}/status"
  assert_success
  refute_output --partial '"state":"ready"'
}

TESTS=( test_79_rejected_apply_does_not_become_ready )
