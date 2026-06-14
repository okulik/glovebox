#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  rm -f "${GBX_CONFIG_DIR}/active-project"
  WS1="$(mktemp -d)"; WS2="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$WS1" >/dev/null
  "${REPO_ROOT}/bin/gbx" new "$WS2" >/dev/null
  PID1="$( _pid_hash "$WS1" )"
  PID2="$( _pid_hash "$WS2" )"
  install -d "${GBX_CONFIG_DIR}/94-state"
  printf '%s\n' "$WS1" > "${GBX_CONFIG_DIR}/94-state/ws1"
  printf '%s\n' "$WS2" > "${GBX_CONFIG_DIR}/94-state/ws2"
  printf '%s\n' "$PID1" > "${GBX_CONFIG_DIR}/94-state/pid1"
  printf '%s\n' "$PID2" > "${GBX_CONFIG_DIR}/94-state/pid2"
}

teardown_file() {
  local pid1 pid2 ws1 ws2
  pid1="$(cat "${GBX_CONFIG_DIR}/94-state/pid1" 2>/dev/null || true)"
  pid2="$(cat "${GBX_CONFIG_DIR}/94-state/pid2" 2>/dev/null || true)"
  ws1="$(cat "${GBX_CONFIG_DIR}/94-state/ws1" 2>/dev/null || true)"
  ws2="$(cat "${GBX_CONFIG_DIR}/94-state/ws2" 2>/dev/null || true)"
  [[ -n "$pid1" ]] && docker rm -f "glovebox-agent-${pid1}" >/dev/null 2>&1 || true
  [[ -n "$pid2" ]] && docker rm -f "glovebox-agent-${pid2}" >/dev/null 2>&1 || true
  rm -rf "$ws1" "$ws2" "${GBX_CONFIG_DIR}/94-state"
}

test_94-gbx-project-ls_lists_both_projects() {
  local pid1 pid2 ws1 ws2
  pid1="$(cat "${GBX_CONFIG_DIR}/94-state/pid1")"
  pid2="$(cat "${GBX_CONFIG_DIR}/94-state/pid2")"
  ws1="$(cat "${GBX_CONFIG_DIR}/94-state/ws1")"
  ws2="$(cat "${GBX_CONFIG_DIR}/94-state/ws2")"
  run "${REPO_ROOT}/bin/gbx" ls
  assert_success
  assert_output --partial "$pid1"
  assert_output --partial "$pid2"
  # Workspace column is capped (currently 65 runes with leading "..." for
  # over-long paths); the leading prefix may be elided. Assert on the
  # basename instead - it's the unique mktemp suffix and always survives.
  assert_output --partial "$(basename "$ws1")"
  assert_output --partial "$(basename "$ws2")"
}

test_94-gbx-project-ls_marks_active_with_asterisk() {
  local pid1 pid2
  pid1="$(cat "${GBX_CONFIG_DIR}/94-state/pid1")"
  pid2="$(cat "${GBX_CONFIG_DIR}/94-state/pid2")"
  # Active is PID1 (first registered; project new only sets default when none exists).
  run "${REPO_ROOT}/bin/gbx" ls
  echo "$output" | grep -E "^\* +${pid1}" || fail "active project not marked"
  echo "$output" | grep -E "^  +${pid2}" || fail "inactive project shouldn't be marked"
}

test_94-gbx-project-ls_shows_agent_state() {
  local pid1
  pid1="$(cat "${GBX_CONFIG_DIR}/94-state/pid1")"
  docker stop "glovebox-agent-${pid1}" >/dev/null
  run "${REPO_ROOT}/bin/gbx" ls
  echo "$output" | grep "$pid1" | grep -q -E "exited|stopped|created" || fail "stopped agent state not reported"
  docker start "glovebox-agent-${pid1}" >/dev/null
}

test_94-gbx-project-ls_verbose_shows_image_after_name() {
  local pid1
  pid1="$(cat "${GBX_CONFIG_DIR}/94-state/pid1")"
  run "${REPO_ROOT}/bin/gbx" ls -v
  assert_success
  assert_output --partial "IMAGE"
  # The agent row must carry its image right after the container name.
  echo "$output" | grep "glovebox-agent-${pid1}" | grep -q "glovebox-agent:local" \
    || fail "agent row missing image name"
}

test_94-gbx-project-ls_json_includes_image() {
  run "${REPO_ROOT}/bin/gbx" ls --json
  assert_success
  assert_output --partial '"image": "glovebox-agent:local"'
}

TESTS=(
  test_94-gbx-project-ls_lists_both_projects
  test_94-gbx-project-ls_marks_active_with_asterisk
  test_94-gbx-project-ls_shows_agent_state
  test_94-gbx-project-ls_verbose_shows_image_after_name
  test_94-gbx-project-ls_json_includes_image
)
