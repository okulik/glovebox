#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  stack_up
}

test_71_unknown_registry_rejected() {
  run curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  x:\n    image: sketchy.example.com/foo:1.0\n' \
    -w '\n%{http_code}' \
    "$(controller_url)/projects/test71/apply"
  assert_success
  assert_output --partial '"error":"image_registry_not_allowed"'
  assert_output --partial 'hint_for_agent'
}

TESTS=( test_71_unknown_registry_rejected )
