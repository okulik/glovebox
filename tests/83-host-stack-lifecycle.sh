#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="hostlife"

setup_file() {
  ensure_env
  stack_up
  curl -sS -X POST "$(controller_url)/projects/${PID}/propose" \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n' >/dev/null
  GBX_PROJECT_ID="$PID" "${REPO_ROOT}/bin/gbx" stack apply -y >/dev/null
}

teardown_file() {
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
}

test_83_ls_shows_project() {
  run "${REPO_ROOT}/bin/gbx" stack ls
  assert_success
  assert_output --partial "$PID"
}

test_83_status_shows_ready() {
  GBX_PROJECT_ID="$PID" run "${REPO_ROOT}/bin/gbx" stack status
  assert_success
  assert_output --partial "ready"
}

test_83_down_stops_redis() {
  GBX_PROJECT_ID="$PID" run "${REPO_ROOT}/bin/gbx" stack down
  assert_success
  run docker ps --filter "name=glovebox-stack-${PID}-redis" --format '{{.Status}}'
  assert_success
  refute_output --partial "Up "
}

TESTS=(
  test_83_ls_shows_project
  test_83_status_shows_ready
  test_83_down_stops_redis
)
