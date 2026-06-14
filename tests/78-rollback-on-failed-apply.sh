#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test78"

setup_file() {
  ensure_env
  stack_up
}

teardown_file() {
  # Defensive cleanup in case anything leaked.
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
  docker rm -f "glovebox-stack-${PID}-redis" "glovebox-stack-${PID}-nope" 2>/dev/null || true
  docker network rm "glovebox-stack-${PID}" 2>/dev/null || true
}

test_78_failed_apply_rolls_back() {
  # One good image, one image whose registry is in the allowlist but whose
  # name does not exist - so the pull fails and apply must roll back.
  local body=$'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n  nope:\n    image: quay.io/does-not-exist-glovebox-rollback/nope:0.1\n'
  local resp http
  resp=$(curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary "$body" -w '\n%{http_code}' \
    "$(controller_url)/projects/${PID}/apply")
  http="${resp##*$'\n'}"
  if [[ "$http" == "200" ]]; then
    fail "expected non-200 from apply, got $http; resp=$resp"
  fi

  # No project network should remain.
  run docker network ls --format '{{.Name}}'
  assert_success
  refute_output --partial "glovebox-stack-${PID}"

  # No containers with this project's prefix should remain.
  run docker ps -a --filter "name=glovebox-stack-${PID}-" --format '{{.Names}}'
  assert_success
  if [[ -n "$output" ]]; then
    fail "leaked containers after rollback: $output"
  fi
}

TESTS=( test_78_failed_apply_rolls_back )
