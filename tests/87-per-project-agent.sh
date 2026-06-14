#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_87_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up >/dev/null
  local ws
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_87_WS_FILE"
}

teardown_file() {
  local ws=""
  [[ -f "$_87_WS_FILE" ]] && ws="$(<"$_87_WS_FILE")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$(_pid_hash "$ws")"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_87_WS_FILE" "${GBX_CONFIG_DIR}/active-project"
}

test_87-per-project-agent_project_new_creates_running_container() {
  local ws pid
  ws="$(<"$_87_WS_FILE")"
  pid="$(_pid_hash "$ws")"
  local state
  state="$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
  assert_equal running "$state"
  docker network inspect glovebox-internal | grep -q "glovebox-agent-${pid}" || fail "agent not attached to internal net"
  local env_pid
  env_pid="$(docker exec "glovebox-agent-${pid}" sh -c 'echo $GBX_PROJECT_ID')"
  assert_equal "$pid" "$env_pid"
}

test_87-per-project-agent_project_start_is_idempotent_after_stop() {
  local ws pid
  ws="$(<"$_87_WS_FILE")"
  pid="$(_pid_hash "$ws")"
  docker stop "glovebox-agent-${pid}" >/dev/null
  run "${REPO_ROOT}/bin/gbx" start "$pid"
  assert_success
  local state
  state="$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
  assert_equal running "$state"
}

TESTS=(
  test_87-per-project-agent_project_new_creates_running_container
  test_87-per-project-agent_project_start_is_idempotent_after_stop
)
