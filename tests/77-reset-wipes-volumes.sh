#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test77"

setup_file() {
  ensure_env
  stack_up
  curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n    volumes:\n      data: /data\n' \
    "$(controller_url)/projects/${PID}/apply" >/dev/null
}

teardown_file() {
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
}

redis_cli_in_stack() {
  docker run --rm --network "glovebox-stack-${PID}" redis:7-alpine \
    redis-cli -h redis "$@"
}

test_77_can_set_value_in_redis() {
  run redis_cli_in_stack SET foo bar
  assert_success
  assert_output --partial "OK"
}

test_77_value_persists_in_volume() {
  redis_cli_in_stack SET persistkey shouldsurvive >/dev/null
  # Force a synchronous save so the RDB snapshot lands on the data volume.
  redis_cli_in_stack SAVE >/dev/null || true
  run redis_cli_in_stack GET persistkey
  assert_success
  assert_output --partial "shouldsurvive"
}

test_77_reset_wipes_data() {
  # Set a key and force it to disk so a regular restart could not lose it.
  redis_cli_in_stack SET willdie yes >/dev/null
  redis_cli_in_stack SAVE >/dev/null || true
  # Reset the service: removes the container and the data volume, recreates both.
  curl -sS -X POST "$(controller_url)/projects/${PID}/services/redis/reset" >/dev/null
  # Give redis time to come back.
  local up=0
  for _ in $(seq 1 20); do
    if redis_cli_in_stack PING 2>/dev/null | grep -q PONG; then up=1; break; fi
    sleep 1
  done
  [[ "$up" -eq 1 ]] || fail "redis did not come back after reset"
  # Key should be gone - GET on a missing key returns an empty string from redis-cli.
  run redis_cli_in_stack GET willdie
  assert_success
  refute_output --partial "yes"
}

TESTS=(
  test_77_can_set_value_in_redis
  test_77_value_persists_in_volume
  test_77_reset_wipes_data
)
