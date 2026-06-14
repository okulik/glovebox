#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_63_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  create_test_workspace "$_63_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_63_WS_FILE"
}

test_63-agent-state-dirs_per_agent_state_directories_exist_on_host_after_gbx_cd() {
  local ws pid
  ws="$(<"$_63_WS_FILE")"
  pid="$( _pid_hash "${ws}" )"
  # Per-project dirs live under state/projects/<pid>/.
  for d in claude codex opencode pi gemini aider hermes; do
    [[ -d "${GBX_CONFIG_DIR}/state/projects/${pid}/${d}" ]] || fail "missing: state/projects/${pid}/${d}"
  done
  # Shared dirs (uv-tools, bin) live under state/shared/.
  for d in uv-tools bin; do
    [[ -d "${GBX_CONFIG_DIR}/state/shared/${d}" ]] || fail "missing: state/shared/${d}"
  done
}

test_63-agent-state-dirs_per_agent_state_dirs_are_owned_by_host_uid_501() {
  local ws pid
  ws="$(<"$_63_WS_FILE")"
  pid="$( _pid_hash "${ws}" )"
  for d in claude codex opencode pi gemini aider hermes; do
    owner="$(stat -f '%u' "${GBX_CONFIG_DIR}/state/projects/${pid}/${d}")"
    [[ "$owner" == "501" ]] || fail "state/projects/${pid}/${d} owner=${owner} (expected 501)"
  done
}

test_63-agent-state-dirs_per_agent_config_dirs_are_mounted_inside_the_container() {
  for path in /home/gbx/.codex /home/gbx/.aider /home/gbx/.local/share/opencode  /home/gbx/.pi /home/gbx/.gemini /home/gbx/.hermes  /home/gbx/.local/share/uv-tools /home/gbx/.local/bin; do
    run in_agent test -d "$path"
    assert_success
  done
}

test_63-agent-state-dirs_aider_history_files_are_redirected_into_the_aider_state_dir() {
  run in_agent bash -lc 'printf "%s\n%s\n" "$AIDER_INPUT_HISTORY_FILE" "$AIDER_CHAT_HISTORY_FILE"'
  assert_success
  assert_line --index 0 "/home/gbx/.aider/.aider.input.history"
  assert_line --index 1 "/home/gbx/.aider/.aider.chat.history.md"
}

test_63-agent-state-dirs_aider_startup_writes_chat_history_in_state_not_workspace() {
  run in_agent bash -lc 'rm -f /workspace/.aider.chat.history.md /home/gbx/.aider/.aider.chat.history.md && aider --list-models sonnet >/dev/null 2>&1 || true'
  assert_success

  run in_agent test -f /home/gbx/.aider/.aider.chat.history.md
  assert_success

  run in_agent test ! -f /workspace/.aider.chat.history.md
  assert_success
}

TESTS=(
  test_63-agent-state-dirs_per_agent_state_directories_exist_on_host_after_gbx_cd
  test_63-agent-state-dirs_per_agent_state_dirs_are_owned_by_host_uid_501
  test_63-agent-state-dirs_per_agent_config_dirs_are_mounted_inside_the_container
  test_63-agent-state-dirs_aider_history_files_are_redirected_into_the_aider_state_dir
  test_63-agent-state-dirs_aider_startup_writes_chat_history_in_state_not_workspace
)
