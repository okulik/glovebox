#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  stack_up
}

test_82-host-cli-stack_help_listed() {
  run "${REPO_ROOT}/bin/gbx" stack
  assert_success
  assert_output --partial "stack apply"
  assert_output --partial "stack down"
}

test_82_apply_brings_up_redis() {
  local pid=hostapply
  curl -sS -X POST "$(controller_url)/projects/${pid}/propose" \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n' >/dev/null
  GBX_PROJECT_ID="$pid" run "${REPO_ROOT}/bin/gbx" stack apply -y
  assert_success
  assert_output --partial '"status":"applied"'
  # The proposal is consumed by a successful apply: a second apply finds none.
  GBX_PROJECT_ID="$pid" run "${REPO_ROOT}/bin/gbx" stack apply -y
  assert_failure
  assert_output --partial "no_proposal"
  curl -sS -X POST "$(controller_url)/projects/${pid}/destroy?confirm=true" >/dev/null
}

test_82_diff_shows_proposed_vs_live() {
  local pid=hostdiff
  curl -sS -X POST "$(controller_url)/projects/${pid}/propose" \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n' >/dev/null
  GBX_PROJECT_ID="$pid" "${REPO_ROOT}/bin/gbx" stack apply -y >/dev/null
  curl -sS -X POST "$(controller_url)/projects/${pid}/propose" \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7.2-alpine\n' >/dev/null
  GBX_PROJECT_ID="$pid" run "${REPO_ROOT}/bin/gbx" stack diff
  assert_success
  assert_output --partial "redis:7-alpine"
  assert_output --partial "redis:7.2-alpine"
  curl -sS -X POST "$(controller_url)/projects/${pid}/destroy?confirm=true" >/dev/null
}

TESTS=(
  test_82-host-cli-stack_help_listed
  test_82_apply_brings_up_redis
  test_82_diff_shows_proposed_vs_live
)
