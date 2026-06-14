#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  stack_up
}

post_manifest() {
  local body="$1"
  curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary "$body" -w '\n%{http_code}' \
    "$(controller_url)/projects/test72/apply"
}

test_72_privileged_rejected() {
  run post_manifest $'version: 1\nservices:\n  x:\n    image: redis:7-alpine\n    privileged: true\n'
  assert_success
  refute_output --partial $'\n200'
  refute_output --partial $'\n202'
}

test_72_cap_add_rejected() {
  run post_manifest $'version: 1\nservices:\n  x:\n    image: redis:7-alpine\n    cap_add: [SYS_ADMIN]\n'
  assert_success
  refute_output --partial $'\n200'
}

test_72_network_mode_host_rejected() {
  run post_manifest $'version: 1\nservices:\n  x:\n    image: redis:7-alpine\n    network_mode: host\n'
  assert_success
  refute_output --partial $'\n200'
}

test_72_host_path_volume_rejected() {
  run post_manifest $'version: 1\nservices:\n  x:\n    image: redis:7-alpine\n    volumes:\n      "/host/path": /data\n'
  assert_success
  refute_output --partial $'\n200'
}

test_72_build_key_rejected() {
  run post_manifest $'version: 1\nservices:\n  x:\n    build: .\n'
  assert_success
  refute_output --partial $'\n200'
}

TESTS=(
  test_72_privileged_rejected
  test_72_cap_add_rejected
  test_72_network_mode_host_rejected
  test_72_host_path_volume_rejected
  test_72_build_key_rejected
)
