#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"



setup_file() {
  ensure_env
  stack_up
}

# A real provider response means curl exited 0 (no transport error). A Squid
# deny shows up as exit 56 with "CONNECT tunnel failed" in stderr - don't be
# fooled by the "403" that appears in the deny message body.
assert_proxy_allowed() {
  [[ "$status" -eq 0 ]] || fail "proxy denied (curl exit=$status, output=$output)"
  [[ "$output" != *"CONNECT tunnel failed"* ]] || fail "Squid denied CONNECT: $output"
}

# For each new provider, probe a representative host and assert a non-000
# status code (any HTTP response proves the proxy let it through).
test_67-allowlist-providers_api_openai_com_is_reachable_via_proxy() {
  run probe_via_proxy "https://api.openai.com/v1/models"
  assert_proxy_allowed
}

test_67-allowlist-providers_openrouter_ai_is_reachable_via_proxy() {
  run probe_via_proxy "https://openrouter.ai/api/v1/models"
  assert_proxy_allowed
}

test_67-allowlist-providers_generativelanguage_googleapis_com_is_reachable_via_proxy() {
  run probe_via_proxy "https://generativelanguage.googleapis.com/"
  assert_proxy_allowed
}

test_67-allowlist-providers_api_deepseek_com_is_reachable_via_proxy() {
  run probe_via_proxy "https://api.deepseek.com/"
  assert_proxy_allowed
}

test_67-allowlist-providers_api_groq_com_is_reachable_via_proxy() {
  run probe_via_proxy "https://api.groq.com/"
  assert_proxy_allowed
}

test_67-allowlist-providers_api_mistral_ai_is_reachable_via_proxy() {
  run probe_via_proxy "https://api.mistral.ai/"
  assert_proxy_allowed
}

TESTS=(
  test_67-allowlist-providers_api_openai_com_is_reachable_via_proxy
  test_67-allowlist-providers_openrouter_ai_is_reachable_via_proxy
  test_67-allowlist-providers_generativelanguage_googleapis_com_is_reachable_via_proxy
  test_67-allowlist-providers_api_deepseek_com_is_reachable_via_proxy
  test_67-allowlist-providers_api_groq_com_is_reachable_via_proxy
  test_67-allowlist-providers_api_mistral_ai_is_reachable_via_proxy
)
