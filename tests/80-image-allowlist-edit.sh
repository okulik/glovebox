#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

PID="test80"
NEW_REGISTRY="other.registry.example"
ALLOWLIST="${REPO_ROOT}/docker/image-allowlist.txt"

setup_file() {
  ensure_env
  stack_up
}

teardown_file() {
  # Always remove the appended registry from the allowlist; restart so the
  # controller's in-memory rules match the file again.
  if [[ -f "$ALLOWLIST" ]]; then
    sed -i.bak "/^${NEW_REGISTRY}$/d" "$ALLOWLIST" || true
    rm -f "${ALLOWLIST}.bak"
  fi
  compose restart stack-controller >/dev/null 2>&1 || true
}

test_80_initially_rejected() {
  local resp
  resp=$(curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary "version: 1
services:
  x:
    image: ${NEW_REGISTRY}/foo:1.0
" -w '\n%{http_code}' "$(controller_url)/projects/${PID}/apply")
  if [[ "$resp" != *"image_registry_not_allowed"* ]]; then
    fail "expected image_registry_not_allowed; got $resp"
  fi
}

test_80_allowlist_extend_and_restart() {
  # Append the new registry to the allowlist file mounted into the controller.
  echo "${NEW_REGISTRY}" >> "$ALLOWLIST"
  # Restart the controller so it re-reads the allowlist at startup.
  compose restart stack-controller >/dev/null
  # Wait for the host listener to be reachable again.
  local ready=0
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -sS --max-time 1 $(controller_url)/health >/dev/null 2>&1; then
      ready=1; break
    fi
    sleep 1
  done
  [[ "$ready" -eq 1 ]] || fail "controller did not come back after restart"

  # The previously-rejected manifest now must NOT fail with
  # image_registry_not_allowed. It may still fail later (image_pull_failed)
  # because the registry doesn't actually exist - that's fine.
  local resp
  resp=$(curl -sS -X POST -H 'Content-Type: text/yaml' \
    --data-binary "version: 1
services:
  x:
    image: ${NEW_REGISTRY}/foo:1.0
" -w '\n%{http_code}' "$(controller_url)/projects/${PID}/apply")
  if [[ "$resp" == *"image_registry_not_allowed"* ]]; then
    fail "registry still not allowed after restart; got $resp"
  fi
}

TESTS=(
  test_80_initially_rejected
  test_80_allowlist_extend_and_restart
)
