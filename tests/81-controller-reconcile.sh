#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test81"

setup_file() {
  ensure_env
  stack_up
  curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n' \
    "$(controller_url)/projects/${PID}/apply" >/dev/null
}

teardown_file() {
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
}

test_81_reconcile_restarts_missing_container() {
  # Drop the redis service container behind the controller's back.
  docker stop "glovebox-stack-${PID}-redis" >/dev/null
  docker rm "glovebox-stack-${PID}-redis" >/dev/null

  # Restart the controller; main() runs state.Reconcile on startup.
  compose restart stack-controller >/dev/null

  # Wait for the controller to be reachable.
  local ready=0
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -sS --max-time 1 $(controller_url)/health >/dev/null 2>&1; then
      ready=1; break
    fi
    sleep 1
  done
  [[ "$ready" -eq 1 ]] || fail "controller did not come back after restart"

  # Wait up to 30s for redis to be Up again, recreated by Reconcile.
  local found=0
  for _ in $(seq 1 30); do
    if docker ps --filter "name=glovebox-stack-${PID}-redis" --format '{{.Status}}' | grep -q "Up"; then
      found=1; break
    fi
    sleep 1
  done

  if [[ "$found" -ne 1 ]]; then
    docker ps -a --filter "name=glovebox-stack-${PID}-" --format '{{.Names}}\t{{.Status}}' >&2
    fail "redis was not reconciled"
  fi
}

TESTS=( test_81_reconcile_restarts_missing_container )
