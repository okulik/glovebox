#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"



setup_file() {
  ensure_env
  stack_up
}

test_05-proxy-allowlist_proxy_allows_api_anthropic_com() {
  run probe_via_proxy "https://api.anthropic.com/"
  assert_success
  # Any non-000 (non-failure) status is fine. We're only proving the CONNECT
  # was permitted and the TLS session opened.
  refute_output "000"
}

test_05-proxy-allowlist_proxy_denies_a_non_allowlisted_domain() {
  run probe_via_proxy "https://example.com/"
  # Squid (with our deny_info override) returns 451 on allowlist-blocked
  # CONNECTs so agents can distinguish a sandbox block from an origin's own
  # 4xx. When curl sees a non-200 on the CONNECT tunnel it exits with code
  # 56 and http_code writes as "000"; stderr contains "response 451".
  # Accept either signal as proof of denial.
  if [[ "$output" == *"000"* ]]; then
    return 0
  fi
  [[ "$output" == *"451"* ]]
}

test_05-proxy-allowlist_block_response_carries_glovebox_egress_header() {
  # `-v` echoes the CONNECT response status and headers. We don't care about
  # the rest of the noise - just confirm the structured marker is there.
  local out
  out="$(docker run --rm --network glovebox-internal curlimages/curl:8.10.1 \
    -sS -v --max-time 8 -x http://proxy:3128 "https://example.com/" 2>&1 || true)"
  [[ "$out" == *"X-Glovebox-Egress"* ]] || fail "expected X-Glovebox-Egress header in denial response; got: $out"
}

test_05-proxy-allowlist_proxy_log_records_the_denial_of_example_com() {
  # Trigger a fresh denial.
  probe_via_proxy "https://example.com/" || true
  run compose exec -T egress-proxy tail -n 20 /var/log/squid/access.log
  assert_success
  assert_output --partial "TCP_DENIED"
  assert_output --partial "example.com"
}

TESTS=(
  test_05-proxy-allowlist_proxy_allows_api_anthropic_com
  test_05-proxy-allowlist_proxy_denies_a_non_allowlisted_domain
  test_05-proxy-allowlist_block_response_carries_glovebox_egress_header
  test_05-proxy-allowlist_proxy_log_records_the_denial_of_example_com
)
