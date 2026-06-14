#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test76"

setup_file() {
  ensure_env
  stack_up
  # Apply a minimal stack so the project network exists.
  curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary $'version: 1\nservices:\n  redis:\n    image: redis:7-alpine\n' \
    "$(controller_url)/projects/${PID}/apply" >/dev/null
}

teardown_file() {
  curl -sS -X POST "$(controller_url)/projects/${PID}/destroy?confirm=true" >/dev/null 2>&1 || true
}

probe_from_stack_net() {
  local url="$1"
  # Attach a one-shot curl container to the project's internal network and probe.
  # Capture both stdout and stderr; on connection refusal curl writes "000" to stdout.
  docker run --rm --network "glovebox-stack-${PID}" curlimages/curl:8.10.1 \
    -sS -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>&1
}

test_76_cannot_reach_public_internet() {
  run probe_from_stack_net "https://1.1.1.1/"
  # Output may include curl errors and a status code. Anything but 200 is OK.
  refute_output "200"
}

test_76_cannot_reach_proxy() {
  run probe_from_stack_net "http://proxy:3128/"
  refute_output "200"
}

TESTS=(
  test_76_cannot_reach_public_internet
  test_76_cannot_reach_proxy
)
