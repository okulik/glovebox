#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_AAD_WS="$(mktemp)"
_AAD_PREBUILD_ID="$(mktemp)"
# Use a throwaway image tag so `gbx rebuild` here doesn't untag the
# operator's real glovebox-agent:local - see GBX_AGENT_IMAGE in
# internal/config/gbx_config.go. PID-suffix keeps parallel runs
# independent.
_AAD_IMAGE="glovebox-agent-test-aad-$$:local"
export GBX_AGENT_IMAGE="$_AAD_IMAGE"

setup_file() {
  ensure_env
  local ws
  ws="$(mktemp -d)"
  # gbx_project_new_to (new + use) rather than bare `gbx new`: the no-arg
  # `gbx rebuild` under test targets the *active* project, and `gbx new`
  # only claims the default slot when none exists. A stale active-project
  # left by another test file would silently redirect the rebuild.
  gbx_project_new_to "$ws"
  printf '%s' "$ws" > "$_AAD_WS"
  # Record the pre-rebuild image ID. After `gbx rebuild` the tag moves to
  # the new image and this one goes dangling; teardown removes it by ID
  # because `--filter reference=glovebox-agent-test-aad-*` no longer
  # matches an untagged image.
  docker images --quiet "$_AAD_IMAGE" 2>/dev/null > "$_AAD_PREBUILD_ID" || true
}

teardown_file() {
  local ws=""
  [[ -f "$_AAD_WS" ]] && ws="$(<"$_AAD_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  rm -f "$_AAD_WS" "${GBX_CONFIG_DIR}/active-project"
  # Drop the test-only tag (post-rebuild image) and the pre-rebuild image
  # captured at setup (now dangling and unmatchable via reference=).
  docker rmi -f "$_AAD_IMAGE" >/dev/null 2>&1 || true
  if [[ -s "$_AAD_PREBUILD_ID" ]]; then
    local prebuild
    prebuild="$(<"$_AAD_PREBUILD_ID")"
    [[ -n "$prebuild" ]] && docker rmi -f "$prebuild" >/dev/null 2>&1 || true
  fi
  rm -f "$_AAD_PREBUILD_ID"
  # Belt and suspenders: any other still-tagged glovebox-agent-test-aad-*
  # from a sibling test or a previous crashed run.
  docker images --filter "reference=glovebox-agent-test-aad-*" -q 2>/dev/null \
    | xargs -r docker rmi -f >/dev/null 2>&1 || true
  unset GBX_AGENT_IMAGE
}

test_AAD-project-rebuild_recreates_target_agent_only() {
  local ws; ws="$(<"$_AAD_WS")"
  local pid
  pid="$( _pid_hash "$ws" )"
  local cid_before
  cid_before="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"

  run "${REPO_ROOT}/bin/gbx" rebuild
  assert_success

  local cid_after
  cid_after="$(docker inspect -f '{{.Id}}' "glovebox-agent-${pid}")"
  [[ "$cid_before" != "$cid_after" ]] || fail "expected container to be recreated"
  assert_equal running "$(docker inspect -f '{{.State.Status}}' "glovebox-agent-${pid}")"
}

test_AAD-project-rebuild_stamps_image_created_label() {
  # The rebuild in the previous test produced a fresh image under the
  # throwaway tag; it must carry the build-time stamp so stale-image
  # situations are diagnosable (each rebuild moves the tag, so a container's
  # label tells which build it is actually running).
  local label
  label="$(docker inspect -f '{{index .Config.Labels "io.glovebox.image.created"}}' "$_AAD_IMAGE")"
  # ISO-8601 UTC with milliseconds, e.g. 2024-09-09T14:16:46.786Z.
  echo "$label" | grep -q -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}Z$' \
    || fail "image label io.glovebox.image.created missing or malformed: '$label'"

  # Containers inherit image labels - that inheritance is what surfaces the
  # stamp in `gbx ls -v`, so pin it down too.
  local ws pid clabel
  ws="$(<"$_AAD_WS")"
  pid="$( _pid_hash "$ws" )"
  clabel="$(docker inspect -f '{{index .Config.Labels "io.glovebox.image.created"}}' "glovebox-agent-${pid}")"
  assert_equal "$label" "$clabel"
}

TESTS=(
  test_AAD-project-rebuild_recreates_target_agent_only
  test_AAD-project-rebuild_stamps_image_created_label
)
