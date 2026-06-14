# Common helpers for all tests.
# Source from each test file when running under the pure-bash runner.

# Resolve repo root once. The runner exports REPO_ROOT, but keep a fallback for
# ad-hoc sourcing.
REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
export REPO_ROOT

# Point gbx at a dedicated test-config subdirectory so all config
# (state/, .env, allowlist.txt) stays out of the repo root.
# Always resolve to an absolute path so Docker Compose volume mounts work correctly
# regardless of whether the caller passed a relative or absolute path.
GBX_CONFIG_DIR="${GBX_CONFIG_DIR:-${REPO_ROOT}/.test-config}"
[[ "$GBX_CONFIG_DIR" == /* ]] || GBX_CONFIG_DIR="$PWD/$GBX_CONFIG_DIR"
export GBX_CONFIG_DIR

# Host port for the stack-controller. 17001 avoids the macOS AFS3 conflict on
# port 7001. Honored by both internal/stack and the test probes that hit
# `http://127.0.0.1:$GBX_CONTROLLER_HOST_PORT/...`.
export GBX_CONTROLLER_HOST_PORT="${GBX_CONTROLLER_HOST_PORT:-17001}"

# Mark every agent container `gbx` creates from inside the test suite with
# `io.glovebox.test=1`. cleanup_glovebox_test_resources wipes by that label
# regardless of whether the test's state dir or workspace dir survived.
export GBX_TEST_MODE=1

# Compute a project id from a workspace path. Mirrors internal/projectid.Hash:
# resolve symlinks, SHA-1 the resolved bytes, take the first 12 hex chars.
# `cd … && pwd -P` is the portable shell equivalent of EvalSymlinks; it also
# requires the path to exist, matching Go's semantics.
_pid_hash() {
  local abs
  abs="$(cd "$1" 2>/dev/null && pwd -P)" || return 1
  printf '%s' "$abs" | shasum -a 1 | cut -c1-12
}

# compose() used to forward to `docker compose -f docker/compose.yml ...`
# back when the stack was compose-defined. The compose.yml is gone now; the
# helper survives as a thin translator so existing tests keep working without
# a rewrite. Supported verbs: up, stop, restart <svc>, exec [-T] <svc> ...,
# ps [--status STATE] [<svc>] [--format FMT].
compose() {
  local verb="${1:-}"
  shift || true
  case "$verb" in
    up)
      "${REPO_ROOT}/bin/gbx" up
      ;;
    stop)
      docker stop glovebox-egress-proxy glovebox-socket-proxy glovebox-stack-controller >/dev/null 2>&1 || true
      ;;
    restart)
      local svc="${1:?compose restart: service name required}"
      docker restart "glovebox-${svc}"
      ;;
    exec)
      # compose's -T (disable TTY) is the default for `docker exec`, so just drop it.
      [[ "${1:-}" == "-T" ]] && shift
      local svc="${1:?compose exec: service name required}"
      shift
      docker exec "glovebox-${svc}" "$@"
      ;;
    ps)
      # Translate the compose-ps shapes the test suite uses into `docker ps`
      # equivalents. Recognized forms:
      #   compose ps [--status STATE] [--format FMT]
      #   compose ps <svc> [--format FMT]
      # `{{.Name}}` (compose) maps to `{{.Names}}` (docker); `{{.Publishers}}`
      # maps to `{{.Ports}}` so publish-state assertions still work.
      local docker_args=("-a")
      local svc_filter=""
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --status)
            docker_args+=(--filter "status=$2")
            shift 2
            ;;
          --format)
            # Bash parameter substitution mangles the curly braces, so use
            # sed for the compose→docker template-field rename.
            local fmt
            fmt="$(printf '%s' "$2" | sed 's/{{\.Name}}/{{.Names}}/g; s/{{\.Publishers}}/{{.Ports}}/g')"
            docker_args+=(--format "$fmt")
            shift 2
            ;;
          --*)
            docker_args+=("$1")
            shift
            ;;
          *)
            svc_filter="$1"
            shift
            ;;
        esac
      done
      if [[ -n "$svc_filter" ]]; then
        docker_args+=(--filter "name=glovebox-${svc_filter}")
      fi
      docker ps "${docker_args[@]}"
      ;;
    *)
      printf 'compose helper: unsupported verb %q\n' "$verb" >&2
      return 2
      ;;
  esac
}

# URL for the stack-controller's host-facing listener.
controller_url() {
  printf 'http://127.0.0.1:%s' "${GBX_CONTROLLER_HOST_PORT:-17001}"
}

# Resolve the active project's agent container name from the active-project
# pointer. Fails the test if no active project is set.
_active_agent_container() {
  local active_file="${GBX_CONFIG_DIR}/active-project"
  [[ -f "$active_file" ]] || fail "no active project; call gbx_project_new_to before in_agent"
  local pid
  pid="$(head -n1 "$active_file")"
  printf 'glovebox-agent-%s' "$pid"
}

# Run a command inside the active project's agent container as the non-root
# user (UID 501 / GID 20 - numeric so it works regardless of the in-image username).
in_agent() {
  local c
  c="$(_active_agent_container)"
  docker exec -u 501:20 -w /workspace "$c" "$@"
}

# Run a command inside the active project's agent container as root (rarely needed).
in_agent_root() {
  local c
  c="$(_active_agent_container)"
  docker exec -u root "$c" "$@"
}

# Convenience: register <path> as a project and set it as the default.
gbx_project_new_to() {
  "${REPO_ROOT}/bin/gbx" new "$1" >/dev/null
  local pid
  pid="$( _pid_hash "$1" )"
  "${REPO_ROOT}/bin/gbx" use "$pid" >/dev/null
}
# Create a temporary workspace, cd into it, and record the path in a state
# file so it persists across subshell boundaries (setup_file runs in a
# subshell; variables set there don't propagate to test functions).
#
# Usage in setup_file:   create_test_workspace "$_TEST_WS_FILE"
# Usage in teardown_file: remove_test_workspace "$_TEST_WS_FILE"
# Usage in tests:         WS="$(cat "$_TEST_WS_FILE")"  or  in_agent ...
#
# _TEST_WS_FILE must be a host path writable by the outer shell. Assign it
# before calling setup_file:
#   _TEST_WS_FILE="$(mktemp)"          # set once at file scope
#   setup_file() { ... create_test_workspace "$_TEST_WS_FILE"; }
create_test_workspace() {
  local state_file="${1:?state_file required}"
  local ws
  ws="$(mktemp -d)"
  gbx_project_new_to "$ws" || { rm -rf "$ws"; return 1; }
  printf '%s' "$ws" > "$state_file"
  printf '%s' "$ws"  # also print for convenience
}

# Remove a test workspace and its per-project agent container.
# Pass the state file written by create_test_workspace.
remove_test_workspace() {
  local state_file="${1:-}"
  local ws=""
  if [[ -n "$state_file" && -f "$state_file" ]]; then
    ws="$(<"$state_file")"
    rm -f "$state_file"
  fi
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" 2>/dev/null || true)"
    if [[ -n "$pid" ]]; then
      docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    fi
    rm -rf "$ws"
  fi
}

# Probe through the proxy from a throwaway curlimages container attached to
# glovebox-internal. Returns curl's exit code; stdout is the HTTP status code.
probe_via_proxy() {
  local url="$1"
  docker run --rm --network glovebox-internal curlimages/curl:8.10.1  -sS -o /dev/null -w "%{http_code}" --max-time 8  -x http://proxy:3128 "$url"
}

# Same, but bypassing the proxy entirely. Used to confirm direct egress is blocked.
probe_no_proxy() {
  local url="$1"
  docker run --rm --network glovebox-internal curlimages/curl:8.10.1  -sS -o /dev/null -w "%{http_code}" --max-time 5 "$url"
}

# Bring the stack up; idempotent. Used in setup_file(). Routes through
# `gbx up`, which in turn calls internal/stack to ensure networks / images /
# the three containers are healthy. The parallel runner sets
# GBX_TESTS_STACK_ALREADY_UP=1 to short-circuit per-file calls.
stack_up() {
  if [[ "${GBX_TESTS_STACK_ALREADY_UP:-}" == "1" ]]; then
    return 0
  fi
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker is not installed or not on PATH; cannot run integration tests"
  fi
  "${REPO_ROOT}/bin/gbx" up
}

# Stop without removing; preserves state mounts. Used in teardown_file().
stack_stop() {
  docker stop glovebox-egress-proxy glovebox-socket-proxy glovebox-stack-controller >/dev/null 2>&1 || true
}

# Remove per-project glovebox containers, networks, and volumes that leaked
# past their test's teardown. Carefully scoped - never touches the user's
# real projects living under a different GBX_CONFIG_DIR. Safe to call
# repeatedly.
#
# Three passes, in order of confidence:
#   1. Label sweep: anything created by a test-mode `gbx` carries
#      `io.glovebox.test=1`. This is the precise, future-proof channel.
#   2. Workspace-orphan scan: legacy. Catches pre-label test containers
#      whose /workspace bind source is under TMPDIR and gone from disk.
#   3. State-dir sweep: derive pids from .test-config* state dirs (plus a
#      hardcoded list for tests that don't go through `gbx new`) and
#      remove the matching agent / stack-network / stack-volume resources.
cleanup_glovebox_test_resources() {
  command -v docker >/dev/null 2>&1 || return 0

  # Pass 1: label sweep. test_helper.sh exports GBX_TEST_MODE=1, which makes
  # every agent container the suite creates carry io.glovebox.test=1. We
  # never label a real user project, so this is safe to be aggressive about.
  docker ps -aq --filter label=io.glovebox.test=1 2>/dev/null \
    | xargs -r docker rm -f >/dev/null 2>&1 || true

  # Pass 2: orphan scan. Resolve the system temp dir to its real path so the
  # comparison works on macOS, where Docker reports bind sources under
  # /private/var/folders/... while $TMPDIR reads as /var/folders/...
  local tmp_root container ws
  tmp_root="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P)" || tmp_root=""
  if [[ -n "$tmp_root" ]]; then
    while IFS= read -r container; do
      [[ -z "$container" ]] && continue
      ws="$(docker inspect "$container" \
              --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}' \
              2>/dev/null)"
      [[ -z "$ws" ]] && continue
      # Must be under the system temp dir (tests use mktemp -d) AND must
      # not exist on disk. Real projects fail at least one of these.
      case "$ws" in
        "$tmp_root"/*|/tmp/*|/private/tmp/*) ;;
        *) continue ;;
      esac
      [[ -d "$ws" ]] && continue
      docker rm -f "$container" >/dev/null 2>&1 || true
    done < <(docker ps -aq --filter name=glovebox-agent- --format '{{.Names}}' 2>/dev/null)
  fi

  # Pass 3: state-dir-derived pid sweep. We deliberately do NOT enumerate
  # $GBX_CONFIG_DIR - a developer may have overridden it to point at their
  # real config.
  local test_pids="" d sub pdir bn
  for d in "${REPO_ROOT}/.test-config" "${REPO_ROOT}"/.test-config.w*; do
    [[ -d "$d" ]] || continue
    for sub in state/projects state/controller; do
      [[ -d "$d/$sub" ]] || continue
      for pdir in "$d/$sub"/*; do
        [[ -d "$pdir" ]] || continue
        bn="$(basename "$pdir")"
        test_pids+="$bn"$'\n'
      done
    done
  done
  # Hardcoded pids used literally by some bash tests.
  test_pids+="test76"$'\n'"test77"$'\n'"test78"$'\n'"test79"$'\n'
  test_pids+="test80"$'\n'"test81"$'\n'"hostapply"$'\n'"hostlife"$'\n'
  # Dedup (bash 3.2 has no associative arrays).
  test_pids="$(printf '%s' "$test_pids" | sort -u)"
  [[ -z "$test_pids" ]] && return 0
  local pid
  while IFS= read -r pid; do
    [[ -z "$pid" ]] && continue
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    docker container ls -a --format '{{.Names}}' 2>/dev/null \
      | grep "^glovebox-stack-${pid}-" \
      | xargs docker rm -f >/dev/null 2>&1 || true
    docker volume ls --format '{{.Name}}' 2>/dev/null \
      | grep "^glovebox-stack-${pid}-" \
      | xargs docker volume rm >/dev/null 2>&1 || true
    docker network rm "glovebox-stack-${pid}" >/dev/null 2>&1 || true
  done <<< "$test_pids"
}

# Ensure .env and allowlist.txt exist; create from examples if not. Tests must
# be runnable on a clean checkout without requiring bootstrap to have been run
# beforehand.
ensure_env() {
  mkdir -p "${GBX_CONFIG_DIR}"
  if [[ ! -f "${GBX_CONFIG_DIR}/.env" ]]; then
    cp "${REPO_ROOT}/.env.example" "${GBX_CONFIG_DIR}/.env"
  fi
  if [[ ! -f "${GBX_CONFIG_DIR}/allowlist.txt" ]]; then
    cp "${REPO_ROOT}/docker/proxy/allowlist.txt" "${GBX_CONFIG_DIR}/allowlist.txt"
  fi
}

# --- Minimal Bats-compatible assertions ------------------------------------

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

skip() {
  printf 'skip: %s\n' "${*:-skipped}" >&2
  exit 200
}

run() {
  local cmd_rc=0 line
  output=""
  lines=()
  set +e
  output="$("$@" 2>&1)"
  cmd_rc=$?
  set -e
  status="$cmd_rc"
  while IFS= read -r line; do
    lines+=("$line")
  done <<<"$output"
  return 0
}

assert_success() {
  [[ "${status:-1}" -eq 0 ]] || fail "expected success, got status=${status:-unset}; output=${output:-}"
}

assert_failure() {
  [[ "${status:-0}" -ne 0 ]] || fail "expected failure, got status=0; output=${output:-}"
}

assert_output() {
  local mode="exact"
  if [[ "${1:-}" == "--partial" ]]; then
    mode="partial"
    shift
  elif [[ "${1:-}" == "--regexp" ]]; then
    mode="regexp"
    shift
  fi

  local expected="$*"
  case "$mode" in
    exact)
      [[ "${output:-}" == "$expected" ]] || fail "output mismatch: expected [$expected], got [${output:-}]"
      ;;
    partial)
      [[ "${output:-}" == *"$expected"* ]] || fail "output missing substring [$expected]; got [${output:-}]"
      ;;
    regexp)
      [[ "${output:-}" =~ $expected ]] || fail "output did not match regex [$expected]; got [${output:-}]"
      ;;
  esac
}

refute_output() {
  local mode="exact"
  if [[ "${1:-}" == "--partial" ]]; then
    mode="partial"
    shift
  elif [[ "${1:-}" == "--regexp" ]]; then
    mode="regexp"
    shift
  fi

  local expected="$*"
  case "$mode" in
    exact)
      [[ "${output:-}" != "$expected" ]] || fail "output unexpectedly matched [$expected]"
      ;;
    partial)
      [[ "${output:-}" != *"$expected"* ]] || fail "output unexpectedly contained [$expected]"
      ;;
    regexp)
      [[ ! "${output:-}" =~ $expected ]] || fail "output unexpectedly matched regex [$expected]"
      ;;
  esac
}

assert_line() {
  local idx=0
  if [[ "${1:-}" == "--index" ]]; then
    idx="$2"
    shift 2
  fi
  local expected="$*"
  [[ "${lines[$idx]-}" == "$expected" ]] || fail "line ${idx} mismatch: expected [$expected], got [${lines[$idx]-}]"
}

refute_line() {
  local idx=0
  if [[ "${1:-}" == "--index" ]]; then
    idx="$2"
    shift 2
  fi
  local unexpected="$*"
  [[ "${lines[$idx]-}" != "$unexpected" ]] || fail "line ${idx} unexpectedly matched [$unexpected]"
}

assert_equal() {
  local expected="$1" actual="$2"
  [[ "$expected" == "$actual" ]] || fail "expected [$expected], got [$actual]"
}

assert_file_contains() {
  local file="$1" needle="$2"
  [[ -f "$file" ]] || fail "expected file: $file"
  grep -qF -- "$needle" "$file" || fail "expected '$needle' in $file"
}

assert_regex() {
  local re="$*"
  [[ "${output:-}" =~ $re ]] || fail "output did not match regex [$re]; got [${output:-}]"
}

refute_regex() {
  local re="$*"
  [[ ! "${output:-}" =~ $re ]] || fail "output unexpectedly matched regex [$re]"
}
