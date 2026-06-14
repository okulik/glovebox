#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

_20_WS_FILE="$(mktemp)"

setup_file() {
  ensure_env
  stack_up
  create_test_workspace "$_20_WS_FILE" >/dev/null
}

teardown_file() {
  remove_test_workspace "$_20_WS_FILE"
}

test_20-no-direct-egress_work_container_has_no_direct_route_to_internet_proxy_bypass_fails() {
  # Exec curl inside the work container with proxy env explicitly unset, so it
  # has no way out except the (absent) default route.
  local c
  c="$(_active_agent_container)"
  run docker exec -u 501:20 "$c" \
    bash -c 'HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= curl -sS -o /dev/null -w "%{http_code}" --max-time 4 https://example.com/ || echo "CURL_FAIL"'
  assert_success
  # Acceptable outputs: contains 000 (connection failure) or CURL_FAIL (curl exited non-zero).
  if [[ "$output" == *"000"* || "$output" == *"CURL_FAIL"* ]]; then
    return 0
  fi
  fail "work container reached the internet directly (output: $output)"
}

test_20-no-direct-egress_work_container_can_reach_internet_through_the_proxy() {
  run in_agent bash -c 'curl -sS -o /dev/null -w "%{http_code}" --max-time 10 https://api.anthropic.com/'
  assert_success
  # Any non-000 status proves the proxy tunneled. 401, 404, 200 all fine.
  # grep for any digit 1-9 to confirm a real HTTP status code came back
  # rather than the all-zeros connection-failure code.
  if ! [[ "$output" =~ [1-9][0-9][0-9] ]]; then
    fail "work container could not reach the internet through the proxy (output: $output)"
  fi
}

TESTS=(
  test_20-no-direct-egress_work_container_has_no_direct_route_to_internet_proxy_bypass_fails
  test_20-no-direct-egress_work_container_can_reach_internet_through_the_proxy
)
