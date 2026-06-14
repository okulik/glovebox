#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

setup_file() {
  ensure_env
  stack_up
}

test_74_apply_404_on_internal_listener() {
  run docker run --rm --network glovebox-internal curlimages/curl:8.10.1 \
    -sS -X POST -o /dev/null -w '%{http_code}' --max-time 5 \
    -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  x:\n    image: redis:7-alpine\n' \
    "http://stack-controller:7000/projects/test74/apply"
  assert_success
  assert_output "404"
}

test_74_list_projects_404_on_internal_listener() {
  run docker run --rm --network glovebox-internal curlimages/curl:8.10.1 \
    -sS -o /dev/null -w '%{http_code}' --max-time 5 \
    "http://stack-controller:7000/projects"
  assert_success
  assert_output "404"
}

TESTS=(
  test_74_apply_404_on_internal_listener
  test_74_list_projects_404_on_internal_listener
)
