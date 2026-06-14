#!/usr/bin/env bash
# Prototype parallel test runner.
#
# Brings the singleton docker compose stack up once, shards parallel-safe
# tests across $WORKERS subprocesses (each with its own GBX_CONFIG_DIR),
# then runs the must-serial bucket against the shared config dir.
#
#   make test-parallel               # WORKERS=4
#   WORKERS=8 make test-parallel
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# WORKERS=2 is the empirical sweet spot on macOS: 134/134 green in ~7min vs
# ~20min serial. WORKERS=4 finishes faster (~5min) but the docker daemon
# starts dropping calls under load (transient `docker inspect`/`exec` failures
# misinterpret "no such container" and the agent flow corrupts itself). If you
# raise this, add retry/backoff in internal/agent.Ensure first.
WORKERS="${WORKERS:-2}"

# Tests that mutate singleton stack state (proxy allowlist, controller-managed
# project state, stack containers) or read state they don't own. They share
# the suite-level GBX_CONFIG_DIR and run sequentially after the parallel phase.
SERIAL_TESTS=(
  tests/05-proxy-allowlist.sh
  tests/10-stack-up.sh
  tests/30-state-survives-restart.sh
  tests/44-wrapper-allow.sh
  tests/65-wrapper-autostart.sh
  tests/67-allowlist-providers.sh
  tests/70-controller-health.sh
  tests/73-apply-roundtrip.sh
  tests/74-host-only-endpoints.sh
  tests/75-agent-cannot-bypass.sh
  tests/76-stack-internal-no-egress.sh
  tests/77-reset-wipes-volumes.sh
  tests/78-rollback-on-failed-apply.sh
  tests/79-wait-shows-rejection.sh
  tests/80-image-allowlist-edit.sh
  tests/81-controller-reconcile.sh
  tests/82-host-cli-stack.sh
  tests/83-host-stack-lifecycle.sh
  tests/84-agent-stack-cli.sh
  tests/93-controller-per-project-attach.sh
)

# Progress helpers. TTY mode uses \r so the status line updates in place;
# non-TTY mode (CI logs, redirects, piping through tee) prints each tick as
# its own line so the log stays readable in retrospect.
TTY=0
[[ -t 1 ]] && TTY=1
status_clear() { (( TTY )) && printf '\r\033[2K' || true; }
banner() { status_clear; printf '==> %s\n' "$*"; }

banner "Building gbx"
make build >/dev/null

# Singleton stack lifecycle. Uses the default suite GBX_CONFIG_DIR.
source "${REPO_ROOT}/tests/test_helper.sh"
ensure_env
banner "Bringing singleton stack up"
stack_up >/dev/null
trap 'status_clear; cleanup_glovebox_test_resources; compose down >/dev/null 2>&1 || true; rm -rf "$GBX_CONFIG_DIR" "${REPO_ROOT}"/.test-config.w*' EXIT

# All discoverable test files minus the serial set and the helper.
ALL_TESTS=()
while IFS= read -r line; do ALL_TESTS+=("$line"); done < <(find tests/ -maxdepth 1 -name '*.sh' ! -name 'test_helper.sh' | sort)
PARALLEL_TESTS=()
for f in "${ALL_TESTS[@]}"; do
  skip=0
  for s in "${SERIAL_TESTS[@]}"; do
    [[ "$f" == "$s" ]] && { skip=1; break; }
  done
  (( skip == 0 )) && PARALLEL_TESTS+=("$f")
done

# Round-robin shard into N buckets.
declare -a BUCKETS
for ((i=0; i<WORKERS; i++)); do BUCKETS[$i]=""; done
i=0
for f in "${PARALLEL_TESTS[@]}"; do
  BUCKETS[$i]="${BUCKETS[$i]} $f"
  i=$(( (i+1) % WORKERS ))
done

banner "Sharding ${#PARALLEL_TESTS[@]} parallel-safe tests across ${WORKERS} workers (${#SERIAL_TESTS[@]} serial after)"

OUTDIR="$(mktemp -d)"
PIDS=()
TOTAL_PER_WORKER=()
for ((i=0; i<WORKERS; i++)); do
  files="${BUCKETS[$i]}"
  count="$(echo "$files" | wc -w | tr -d ' ')"
  TOTAL_PER_WORKER+=("$count")
  [[ -z "${files// /}" ]] && continue
  (
    export GBX_CONFIG_DIR="${REPO_ROOT}/.test-config.w${i}"
    # The singleton stack is already up under the suite config dir; tell the
    # per-file stack_up calls to short-circuit so workers don't try to re-up
    # compose with their own (per-worker) mount sources.
    export GBX_TESTS_STACK_ALREADY_UP=1
    rm -rf "$GBX_CONFIG_DIR"
    mkdir -p "$GBX_CONFIG_DIR"
    cp "${REPO_ROOT}/.env.example" "$GBX_CONFIG_DIR/.env"
    cp "${REPO_ROOT}/docker/proxy/allowlist.txt" "$GBX_CONFIG_DIR/allowlist.txt"
    # shellcheck disable=SC2086
    "${REPO_ROOT}/scripts/run-tests.sh" $files > "$OUTDIR/w${i}.out" 2>&1
    echo $? > "$OUTDIR/w${i}.rc"
  ) &
  PIDS+=($!)
done

# Progress loop. Spinner is braille "dots" (10 frames). It animates at 5fps
# in TTY mode by rewriting the same line via \r + clear-EOL; non-TTY mode
# emits a fresh line every 10s instead. The worker-progress counter ("files
# done" per worker) is recomputed only every ~1s to avoid grep'ing log files
# five times a second.
SPIN=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
spin_idx=0
start=$SECONDS
status=''
tick=0
last_log_line=-1
while true; do
  # Recompute the "files done" / alive count once per second.
  if (( tick % 5 == 0 )); then
    alive=0
    status=''
    for ((i=0; i<WORKERS; i++)); do
      [[ ${TOTAL_PER_WORKER[$i]} -eq 0 ]] && continue
      if kill -0 "${PIDS[$i]}" 2>/dev/null; then alive=$((alive+1)); fi
      # Each test file ends with a `# file-done <path>` marker emitted by
      # run-tests.sh. The end-of-run `# pass=…` summary fires only once
      # per invocation, which is why earlier versions of this loop sat at
      # 0/N the whole time. grep -c also prints "0" AND exits 1 on no
      # matches; capturing with `|| true` avoids the doubled-output bug.
      done_count=0
      if [[ -f "$OUTDIR/w${i}.out" ]]; then
        c="$(grep -c '^# file-done ' "$OUTDIR/w${i}.out" 2>/dev/null || true)"
        done_count="${c:-0}"
      fi
      status+="w${i} ${done_count}/${TOTAL_PER_WORKER[$i]}  "
    done
  fi
  elapsed=$((SECONDS - start))
  spin="${SPIN[$spin_idx]}"
  spin_idx=$(( (spin_idx + 1) % ${#SPIN[@]} ))
  if (( TTY )); then
    printf '\r\033[2K %s [%02d:%02d]  %s' "$spin" $((elapsed/60)) $((elapsed%60)) "$status"
  else
    # Non-TTY: emit a fresh line every 10s; deduped so we don't double-print
    # within a single elapsed-second bucket.
    if (( elapsed > 0 && elapsed % 10 == 0 && elapsed != last_log_line )); then
      printf '   [%02d:%02d]  %s\n' $((elapsed/60)) $((elapsed%60)) "$status"
      last_log_line=$elapsed
    fi
  fi
  (( alive == 0 )) && break
  tick=$((tick+1))
  sleep 0.2
done
status_clear
banner "Parallel workers done after $((SECONDS - start))s"

for pid in "${PIDS[@]}"; do wait "$pid" || true; done

RC=0
for ((i=0; i<WORKERS; i++)); do
  [[ -f "$OUTDIR/w${i}.out" ]] || continue
  printf '===== worker %d =====\n' "$i"
  cat "$OUTDIR/w${i}.out"
  if [[ -f "$OUTDIR/w${i}.rc" ]] && [[ "$(<"$OUTDIR/w${i}.rc")" != "0" ]]; then RC=1; fi
done

banner "Running ${#SERIAL_TESTS[@]} must-serial tests"
serial_start=$SECONDS
"${REPO_ROOT}/scripts/run-tests.sh" "${SERIAL_TESTS[@]}" || RC=1
banner "Serial bucket done after $((SECONDS - serial_start))s; total wall-time: $((SECONDS - start))s"

rm -rf "$OUTDIR"
exit $RC
