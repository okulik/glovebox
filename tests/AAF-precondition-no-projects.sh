#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  rm -rf "${GBX_CONFIG_DIR}/state/projects"
  rm -f "${GBX_CONFIG_DIR}/active-project"
}

test_AAF_run_refuses_with_no_projects() {
  run "${REPO_ROOT}/bin/gbx" run -- true
  assert_failure
  assert_output --partial "No projects. Run 'gbx new <path>' first."
}

test_AAF_project_ls_works_with_no_projects() {
  run "${REPO_ROOT}/bin/gbx" ls
  assert_success
}

test_AAF_project_new_works_with_no_projects() {
  local ws; ws="$(mktemp -d)"
  run "${REPO_ROOT}/bin/gbx" new "$ws"
  assert_success
  local pid
  pid="$( _pid_hash "$ws" )"
  docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
  rm -rf "$ws"
}

TESTS=(
  test_AAF_run_refuses_with_no_projects
  test_AAF_project_ls_works_with_no_projects
  test_AAF_project_new_works_with_no_projects
)
