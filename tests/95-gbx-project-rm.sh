#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
}

test_95-gbx-project-rm_default_preserves_state_dir() {
  local ws pid
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  pid="$( _pid_hash "$ws" )"
  install -d "${GBX_CONFIG_DIR}/state/projects/${pid}/claude"
  echo "marker" > "${GBX_CONFIG_DIR}/state/projects/${pid}/claude/marker"

  run "${REPO_ROOT}/bin/gbx" rm "$pid" --yes
  assert_success
  ! docker container inspect "glovebox-agent-${pid}" >/dev/null 2>&1  || fail "container still exists"
  [[ -d "${GBX_CONFIG_DIR}/state/projects/${pid}" ]] || fail "state dir should be preserved by default"
  [[ -f "${GBX_CONFIG_DIR}/state/projects/${pid}/claude/marker" ]] || fail "claude marker file missing"
  rm -rf "$ws" "${GBX_CONFIG_DIR}/state/projects/${pid}"
}

test_95-gbx-project-rm_delete_state_flag_wipes_dir() {
  local ws pid
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  pid="$( _pid_hash "$ws" )"
  run "${REPO_ROOT}/bin/gbx" rm "$pid" --delete-state --yes
  assert_success
  [[ ! -d "${GBX_CONFIG_DIR}/state/projects/${pid}" ]] || fail "state dir still exists after --delete-state"
  rm -rf "$ws"
}

test_95-gbx-project-rm_prefix_resolves_uniquely() {
  local ws1 pid1
  ws1="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  pid1="$( _pid_hash "$ws1" )"
  local prefix="${pid1:0:6}"
  run "${REPO_ROOT}/bin/gbx" rm "$prefix" --yes
  assert_success
  rm -rf "$ws1" "${GBX_CONFIG_DIR}/state/projects/${pid1}"
}

test_95-gbx-project-rm_ambiguous_prefix_errors() {
  # Manually create two state dirs that share a prefix.
  install -d "${GBX_CONFIG_DIR}/state/projects/abcd11111111"  "${GBX_CONFIG_DIR}/state/projects/abcd22222222"
  run "${REPO_ROOT}/bin/gbx" rm "abcd" --yes
  assert_failure
  assert_output --partial "ambiguous"
  rm -rf "${GBX_CONFIG_DIR}/state/projects/abcd11111111"  "${GBX_CONFIG_DIR}/state/projects/abcd22222222"
}

TESTS=(
  test_95-gbx-project-rm_default_preserves_state_dir
  test_95-gbx-project-rm_delete_state_flag_wipes_dir
  test_95-gbx-project-rm_prefix_resolves_uniquely
  test_95-gbx-project-rm_ambiguous_prefix_errors
)
