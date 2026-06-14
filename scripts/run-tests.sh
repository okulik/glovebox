#!/usr/bin/env bash
# Pure-bash test runner for the glovebox suite.
#
# It understands the subset of test syntax used in this repository:
# - setup_file / teardown_file / setup / teardown functions
# - TESTS=(...) arrays listing test functions
# - run / assert_* / refute_* / skip / fail helpers from tests/test_helper.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Build the Go binary before the bash tests so they invoke a fresh build.
make build >/dev/null

# Make the helper available to each test file.
source "${REPO_ROOT}/tests/test_helper.sh"

shopt -s nullglob

# In full-suite mode, do stack setup/teardown once at the suite boundary so
# the ~16 files that used to `compose down` between tests don't churn the
# stack repeatedly. Per-file `stack_up` calls remain (idempotent) so single-
# file runs via `make test FILE=…` still work.
SUITE_MODE=1
if [[ $# -gt 0 ]]; then
  SUITE_MODE=0
fi

TEST_FILES=()
while IFS= read -r f; do
  TEST_FILES+=("$f")
done < <(find tests/ -maxdepth 1 -name '*.sh' | sort)

if [[ ${#TEST_FILES[@]} -eq 0 ]]; then
  echo "No test files found."
  exit 0
fi

# Optional CLI filter:
#   ./scripts/run-tests.sh tests/41-wrapper-cd.sh
#   make test FILE=tests/41-wrapper-cd.sh
if [[ $# -gt 0 ]]; then
  REQUESTED_FILES=()
  for arg in "$@"; do
    if [[ -f "$arg" ]]; then
      REQUESTED_FILES+=("$arg")
    elif [[ -f "tests/$arg" ]]; then
      REQUESTED_FILES+=("tests/$arg")
    else
      echo "Unknown test file: $arg" >&2
      exit 2
    fi
  done
  TEST_FILES=("${REQUESTED_FILES[@]}")
fi

if [[ $SUITE_MODE -eq 1 ]]; then
  ensure_env
  stack_up >/dev/null
fi

pass=0
fail=0
skip=0
count=0

run_file_scope_hook() {
  local hook_name="$1"
  if declare -F "$hook_name" >/dev/null 2>&1; then
    local tmp_out rc
    tmp_out="$(mktemp)"
    set +e
    ( set -e; "$hook_name" ) >"$tmp_out" 2>&1
    rc=$?
    set -e
    HOOK_OUTPUT="$(<"$tmp_out")"
    rm -f "$tmp_out"
    return "$rc"
  fi
  return 0
}

hook_status() {
  local hook_name="$1"
  local rc=0
  run_file_scope_hook "$hook_name"
  rc=$?
  return "$rc"
}

run_test_case() {
  local test_name="$1"
  local tmp_out rc setup_rc test_rc teardown_rc
  tmp_out="$(mktemp)"
  setup_rc=0
  test_rc=0
  teardown_rc=0

  set +e
  (
    set +e
    if declare -F setup >/dev/null 2>&1; then
      setup
      setup_rc=$?
    fi

    if [[ $setup_rc -eq 0 ]]; then
      "$test_name"
      test_rc=$?
    else
      test_rc=$setup_rc
    fi

    if declare -F teardown >/dev/null 2>&1; then
      teardown
      teardown_rc=$?
    fi

    if [[ $setup_rc -ne 0 ]]; then
      exit "$setup_rc"
    fi
    if [[ $test_rc -ne 0 ]]; then
      exit "$test_rc"
    fi
    exit "$teardown_rc"
  ) >"$tmp_out" 2>&1
  rc=$?
  set -e

  TEST_OUTPUT="$(<"$tmp_out")"
  rm -f "$tmp_out"

  if [[ $rc -eq 200 ]]; then
    return 200
  fi
  return "$rc"
}

for src in ${TEST_FILES[@]+"${TEST_FILES[@]}"}; do
  TESTS=()
  # Clear hook functions from the previous file so they don't bleed into this one.
  unset -f setup_file teardown_file setup teardown 2>/dev/null || true
  source "$src"

  if run_file_scope_hook setup_file; then
    rc=0
  else
    rc=$?
    if [[ $rc -eq 200 ]]; then
      echo "ok - $src # SKIP"
      skip=$((skip + 1))
      # still attempt teardown_file
      run_file_scope_hook teardown_file >/dev/null 2>&1 || true
      continue
    fi
    printf 'not ok - %s (setup_file failed)\n%s\n' "$src" "$HOOK_OUTPUT" >&2
    fail=$((fail + 1))
    run_file_scope_hook teardown_file >/dev/null 2>&1 || true
    continue
  fi

  for test_name in ${TESTS[@]+"${TESTS[@]}"}; do
    count=$((count + 1))
    if run_test_case "$test_name"; then
      echo "ok $count - $test_name"
      pass=$((pass + 1))
    else
      rc=$?
      if [[ $rc -eq 200 ]]; then
        echo "ok $count - $test_name # SKIP"
        skip=$((skip + 1))
      else
        printf 'not ok %d - %s\n' "$count" "$test_name" >&2
        if [[ -n "${TEST_OUTPUT:-}" ]]; then
          printf '%s\n' "$TEST_OUTPUT" >&2
        fi
        fail=$((fail + 1))
      fi
    fi
  done

  if run_file_scope_hook teardown_file; then
    rc=0
  else
    rc=$?
    if [[ $rc -eq 200 ]]; then
      echo "ok - $src # SKIP teardown"
      skip=$((skip + 1))
    else
      printf 'not ok - %s (teardown_file failed)\n%s\n' "$src" "$HOOK_OUTPUT" >&2
      fail=$((fail + 1))
    fi
  fi

  # Per-file completion marker. parallel-tests.sh tails the worker logs and
  # counts these to show "files done / total" in its spinner. Cheap and
  # invisible to the human-readable TAP stream.
  printf '# file-done %s\n' "$src"
done

if [[ $SUITE_MODE -eq 1 ]]; then
  # Sweep per-project glovebox resources that individual tests' teardowns may
  # have skipped (e.g. mid-test crashes). Preserves the singleton compose
  # services so `compose down` below shuts them down cleanly.
  cleanup_glovebox_test_resources
  compose down >/dev/null 2>&1 || true
  # Wipe the test config dir so successive runs start from a known state.
  # Filter mode (`make test FILE=...`) skips this so failing single-file runs
  # leave state around for inspection.
  rm -rf "$GBX_CONFIG_DIR"
fi

printf '1..%d\n' "$count"
printf '# pass=%d fail=%d skip=%d\n' "$pass" "$fail" "$skip"

[[ $fail -eq 0 ]]
