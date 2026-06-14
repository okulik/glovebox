#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAI_WS1="$(mktemp)"
_AAI_WS2="$(mktemp)"
_AAI_HOST_MOUNT="$(mktemp)"

setup_file() {
  ensure_env
  local ws1 ws2 host_mount
  ws1="$(mktemp -d)"
  ws2="$(mktemp -d)"
  host_mount="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws1" >/dev/null
  "${REPO_ROOT}/bin/gbx" new "$ws2" >/dev/null
  # Project 1 (ws1) is the active project (first registered).
  printf '%s' "$ws1" > "$_AAI_WS1"
  printf '%s' "$ws2" > "$_AAI_WS2"
  printf '%s' "$host_mount" > "$_AAI_HOST_MOUNT"
}

teardown_file() {
  local ws1="" ws2="" host=""
  [[ -f "$_AAI_WS1" ]] && ws1="$(<"$_AAI_WS1")"
  [[ -f "$_AAI_WS2" ]] && ws2="$(<"$_AAI_WS2")"
  [[ -f "$_AAI_HOST_MOUNT" ]] && host="$(<"$_AAI_HOST_MOUNT")"
  if [[ -n "$ws1" ]]; then
    docker rm -f "glovebox-agent-$( _pid_hash "$ws1" )" >/dev/null 2>&1 || true
    rm -rf "$ws1"
  fi
  if [[ -n "$ws2" ]]; then
    docker rm -f "glovebox-agent-$( _pid_hash "$ws2" )" >/dev/null 2>&1 || true
    rm -rf "$ws2"
  fi
  [[ -n "$host" ]] && rm -rf "$host" || true
  rm -f "$_AAI_WS1" "$_AAI_WS2" "$_AAI_HOST_MOUNT" "${GBX_CONFIG_DIR}/active-project"
}

test_AAI-mount-add_writes_mounts_file() {
  local ws1 host pid1 mfile
  ws1="$(<"$_AAI_WS1")"
  host="$(<"$_AAI_HOST_MOUNT")"
  pid1="$( _pid_hash "$ws1" )"
  mfile="${GBX_CONFIG_DIR}/state/projects/${pid1}/mounts.txt"

  run "${REPO_ROOT}/bin/gbx" -p "$pid1" mount add "${host}:/mnt/extra:ro"
  assert_success
  [[ -f "$mfile" ]] || fail "expected mounts.txt at $mfile"
  # The recorded host is symlink-resolved.
  local resolved
  resolved="$(cd "$host" && pwd -P)"
  assert_file_contains "$mfile" "${resolved}:/mnt/extra:ro"
}

test_AAI-mount-ls_prints_current_set() {
  local pid1
  pid1="$( _pid_hash "$(<"$_AAI_WS1")" )"
  run "${REPO_ROOT}/bin/gbx" -p "$pid1" mount ls
  assert_success
  assert_output --partial ":/mnt/extra:ro"
}

test_AAI-mount-apply_recreates_container_with_new_volume() {
  local ws1 host pid1 host_resolved cname cid_before cid_after
  ws1="$(<"$_AAI_WS1")"
  host="$(<"$_AAI_HOST_MOUNT")"
  pid1="$( _pid_hash "$ws1" )"
  host_resolved="$(cd "$host" && pwd -P)"
  cname="glovebox-agent-${pid1}"
  cid_before="$(docker inspect -f '{{.Id}}' "$cname")"

  run "${REPO_ROOT}/bin/gbx" -p "$pid1" mount apply
  assert_success

  cid_after="$(docker inspect -f '{{.Id}}' "$cname")"
  [[ "$cid_before" != "$cid_after" ]] || fail "expected container to be recreated"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "$cname")"

  # Verify the new bind mount is attached and read-only.
  local mounts_json
  mounts_json="$(docker inspect -f '{{json .Mounts}}' "$cname")"
  [[ "$mounts_json" == *"${host_resolved}"* ]] || fail "container missing host mount source ${host_resolved}; mounts=${mounts_json}"
  [[ "$mounts_json" == *"/mnt/extra"* ]] || fail "container missing /mnt/extra; mounts=${mounts_json}"
}

test_AAI-mount-rm_removes_entry() {
  local ws1 host pid1 mfile resolved
  ws1="$(<"$_AAI_WS1")"
  host="$(<"$_AAI_HOST_MOUNT")"
  pid1="$( _pid_hash "$ws1" )"
  resolved="$(cd "$host" && pwd -P)"
  mfile="${GBX_CONFIG_DIR}/state/projects/${pid1}/mounts.txt"

  run "${REPO_ROOT}/bin/gbx" -p "$pid1" mount rm "/mnt/extra"
  assert_success
  if grep -qF "${resolved}:/mnt/extra:ro" "$mfile" 2>/dev/null; then
    fail "entry still present in $mfile after rm"
  fi
}

test_AAI-mount-p-flag_routes_to_other_project() {
  local ws2 host pid1 pid2
  ws2="$(<"$_AAI_WS2")"
  host="$(<"$_AAI_HOST_MOUNT")"
  pid1="$( _pid_hash "$(<"$_AAI_WS1")" )"
  pid2="$( _pid_hash "$ws2" )"

  # Add to project 2 via -p; project 1 must remain empty.
  run "${REPO_ROOT}/bin/gbx" -p "$pid2" mount add "${host}:/mnt/p2"
  assert_success

  run "${REPO_ROOT}/bin/gbx" -p "$pid2" mount ls
  assert_success
  assert_output --partial ":/mnt/p2:rw"

  run "${REPO_ROOT}/bin/gbx" -p "$pid1" mount ls
  assert_success
  refute_output --partial ":/mnt/p2:"
}

TESTS=(
  test_AAI-mount-add_writes_mounts_file
  test_AAI-mount-ls_prints_current_set
  test_AAI-mount-apply_recreates_container_with_new_volume
  test_AAI-mount-rm_removes_entry
  test_AAI-mount-p-flag_routes_to_other_project
)
