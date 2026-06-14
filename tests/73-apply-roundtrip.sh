#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

# State files shared across setup_file / teardown_file / test functions
# (subshell boundaries mean variables don't propagate, so we use temp files).
_73_WS_FILE="$(mktemp)"
_73_PID_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  # Create a real workspace so gbx new spins up glovebox-agent-<pid>.
  create_test_workspace "$_73_WS_FILE" >/dev/null
  # Derive and persist the pid so tests and teardown can reference it.
  local ws pid
  ws="$(<"$_73_WS_FILE")"
  pid="$(_pid_hash "$ws")"
  printf '%s' "$pid" > "$_73_PID_FILE"
}

teardown_file() {
  local pid=""
  [[ -f "$_73_PID_FILE" ]] && pid="$(<"$_73_PID_FILE")"
  # Destroy the project stack via the controller.
  if [[ -n "$pid" ]]; then
    curl -sS -X POST "$(controller_url)/projects/${pid}/destroy?confirm=true" >/dev/null 2>&1 || true
  fi
  remove_test_workspace "$_73_WS_FILE"
  rm -f "$_73_PID_FILE"
}

test_73-apply-roundtrip_apply_brings_up_redis() {
  local pid body
  pid="$(<"$_73_PID_FILE")"
  body=$(cat <<'EOF'
version: 1
services:
  redis:
    image: redis:7-alpine
EOF
)
  run curl -sS -X POST -H 'Content-Type: text/yaml'  --data-binary "$body"  -w '\nHTTP:%{http_code}'  "$(controller_url)/projects/${pid}/apply"
  assert_success
  assert_output --partial 'HTTP:200'
  assert_output --partial '"status":"applied"'
}

test_73-apply-roundtrip_network_exists() {
  local pid
  pid="$(<"$_73_PID_FILE")"
  run docker network ls --format '{{.Name}}'
  assert_success
  assert_output --partial "glovebox-stack-${pid}"
}

test_73-apply-roundtrip_redis_container_running() {
  local pid
  pid="$(<"$_73_PID_FILE")"
  run docker ps --filter "name=glovebox-stack-${pid}-redis" --format '{{.Names}}\t{{.Status}}'
  assert_success
  assert_output --partial "glovebox-stack-${pid}-redis"
  assert_output --partial "Up"
}

test_73-apply-roundtrip_controller_attaches_agent_to_stack_network() {
  # Apply should have connected glovebox-agent-<pid> to the project's stack
  # network so the agent can resolve service DNS names.
  local pid
  pid="$(<"$_73_PID_FILE")"
  run docker network inspect "glovebox-stack-${pid}"  --format '{{range .Containers}}{{.Name}} {{end}}'
  assert_success
  assert_output --partial 'glovebox-agent'
}

test_73-apply-roundtrip_agent_resolves_redis_by_dns() {
  run in_agent sh -c "getent hosts redis | awk '{print \$1}'"
  assert_success
  refute_output ""
}

TESTS=(
  test_73-apply-roundtrip_apply_brings_up_redis
  test_73-apply-roundtrip_network_exists
  test_73-apply-roundtrip_redis_container_running
  test_73-apply-roundtrip_controller_attaches_agent_to_stack_network
  test_73-apply-roundtrip_agent_resolves_redis_by_dns
)
