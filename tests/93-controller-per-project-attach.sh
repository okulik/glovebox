#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  WS="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$WS" >/dev/null
  PID="$( _pid_hash "$WS" )"
  curl -sS -X POST "$(controller_url)/projects/${PID}/propose" \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n    volumes:\n      data: /data\n' >/dev/null
  install -d "${GBX_CONFIG_DIR}/93-state"
  printf '%s\n' "$WS" > "${GBX_CONFIG_DIR}/93-state/ws"
  printf '%s\n' "$PID" > "${GBX_CONFIG_DIR}/93-state/pid"
}

teardown_file() {
  local pid ws
  pid="$(cat "${GBX_CONFIG_DIR}/93-state/pid" 2>/dev/null || true)"
  ws="$(cat "${GBX_CONFIG_DIR}/93-state/ws" 2>/dev/null || true)"
  if [[ -n "$pid" ]]; then
    GBX_PROJECT_ID="$pid" GBX_PROJECT_DIR="${ws:-/tmp}"  "${REPO_ROOT}/bin/gbx" stack destroy --yes >/dev/null 2>&1 || true
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
  fi
  rm -rf "$ws" "${GBX_CONFIG_DIR}/93-state"
}

test_93-controller-per-project-attach_apply_attaches_pid_agent_to_stack_net() {
  local pid ws
  pid="$(cat "${GBX_CONFIG_DIR}/93-state/pid")"
  ws="$(cat "${GBX_CONFIG_DIR}/93-state/ws")"

  run env GBX_PROJECT_ID="$pid" GBX_PROJECT_DIR="$ws"  "${REPO_ROOT}/bin/gbx" stack apply --yes
  assert_success

  # The per-project agent must be on glovebox-stack-<pid>.
  docker network inspect "glovebox-stack-${pid}"  | grep -q "glovebox-agent-${pid}"  || fail "agent ${pid} not attached to stack network"

  # Redis must be reachable by DNS from inside the agent (python3 socket probe).
  run docker exec "glovebox-agent-${pid}"  python3 -c "import socket; s=socket.create_connection(('redis', 6379), 2); s.send(b'PING\r\n'); data=s.recv(64); s.close(); print(data.decode())"
  assert_output --partial "PONG"
}

TESTS=(
  test_93-controller-per-project-attach_apply_attaches_pid_agent_to_stack_net
)
