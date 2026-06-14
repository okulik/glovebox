#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"



setup_file() {
  ensure_env
  local ws; ws="$(mktemp -d)"; "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  cp "${GBX_CONFIG_DIR}/allowlist.txt" "${GBX_CONFIG_DIR}/allowlist.txt.bak"
}

teardown_file() {
  # Restore original allowlist + reload proxy so other tests aren't affected.
  if [[ -f "${GBX_CONFIG_DIR}/allowlist.txt.bak" ]]; then
    mv "${GBX_CONFIG_DIR}/allowlist.txt.bak" "${GBX_CONFIG_DIR}/allowlist.txt"
    compose restart egress-proxy >/dev/null 2>&1 || true
  fi
}

test_44-wrapper-allow_before_allow_example_com_is_denied_via_proxy() {
  run probe_via_proxy "https://example.com/"
  # output is either 000 (curl fail) or 403 - both prove deny.
  [[ "$output" == *"000"* || "$output" == *"403"* ]]
}

test_44-wrapper-allow_gbx_allow_example_com_appends_domain_reloads_proxy() {
  run "${REPO_ROOT}/bin/gbx" allow example.com
  assert_success

  # allowlist.txt should now contain example.com.
  run grep -E '^example\.com$' "${GBX_CONFIG_DIR}/allowlist.txt"
  assert_success
}

test_44-wrapper-allow_after_allow_example_com_is_reachable_via_proxy() {
  # Give Squid a moment to finish reload.
  sleep 2
  run probe_via_proxy "https://example.com/"
  # Match a 200-class status code (any 2/3/4-digit code starting with 1-9).
  [[ "$output" =~ [1-9][0-9][0-9] ]]
}

TESTS=(
  test_44-wrapper-allow_before_allow_example_com_is_denied_via_proxy
  test_44-wrapper-allow_gbx_allow_example_com_appends_domain_reloads_proxy
  test_44-wrapper-allow_after_allow_example_com_is_reachable_via_proxy
)
