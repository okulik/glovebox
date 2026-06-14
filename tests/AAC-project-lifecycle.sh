#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAC_WS="$(mktemp)"

setup_file() {
  ensure_env
  local ws
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_AAC_WS"
}

teardown_file() {
  local ws=""
  [[ -f "$_AAC_WS" ]] && ws="$(<"$_AAC_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_AAC_WS" "${GBX_CONFIG_DIR}/active-project"
}

_AAC_pid() {
  local ws; ws="$(<"$_AAC_WS")"
  _pid_hash "$ws"
}

test_AAC-project-stop_then_start_brings_agent_back() {
  local pid; pid="$(_AAC_pid)"

  run "${REPO_ROOT}/bin/gbx" stop
  assert_success
  local st
  st="$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
  [[ "$st" == "exited" || "$st" == "created" ]] || fail "expected stopped, got $st"

  run "${REPO_ROOT}/bin/gbx" start
  assert_success
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
}

test_AAC-project-restart_keeps_same_container_id() {
  local pid; pid="$(_AAC_pid)"
  local before
  before="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"

  run "${REPO_ROOT}/bin/gbx" restart
  assert_success

  local after
  after="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"
  assert_equal "$before" "$after"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
}

test_AAC-project-start_with_explicit_id_works() {
  local pid; pid="$(_AAC_pid)"
  "${REPO_ROOT}/bin/gbx" stop >/dev/null

  run "${REPO_ROOT}/bin/gbx" start "${pid:0:6}"
  assert_success
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
}

TESTS=(
  test_AAC-project-stop_then_start_brings_agent_back
  test_AAC-project-restart_keeps_same_container_id
  test_AAC-project-start_with_explicit_id_works
)
