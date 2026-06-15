#!/usr/bin/env bash
source "${REPO_ROOT}/tests/test_helper.sh"

# The four test cases share one workspace and run in declared order
# (add -> ls -> rebuild -> rm); each relies on the previous one's on-disk state.

_AAJ_WS="$(mktemp)"
_AAJ_EDITOR="$(mktemp)"

setup_file() {
  ensure_env
  local ws editor
  ws="$(mktemp -d)"
  "${REPO_ROOT}/bin/gbx" new "$ws" >/dev/null
  printf '%s' "$ws" > "$_AAJ_WS"

  # A stub editor: overwrites the file it's given with a known valid fragment.
  editor="$(mktemp -d)/stub-editor.sh"
  cat > "$editor" <<'EOS'
#!/usr/bin/env bash
cat > "$1" <<'FRAG'
# gbx:description: integration test plugin
RUN true
FRAG
EOS
  chmod +x "$editor"
  printf '%s' "$editor" > "$_AAJ_EDITOR"
}

teardown_file() {
  local ws="" editor=""
  [[ -f "$_AAJ_WS" ]] && ws="$(<"$_AAJ_WS")"
  if [[ -n "$ws" ]]; then
    local pid
    pid="$( _pid_hash "$ws" )"
    docker rm -f "glovebox-agent-${pid}" >/dev/null 2>&1 || true
    docker rmi -f "glovebox-agent-${pid}:local" >/dev/null 2>&1 || true
    rm -rf "$ws"
  fi
  # The stub editor lives under its own `mktemp -d`; remove that dir, not just the script.
  [[ -f "$_AAJ_EDITOR" ]] && editor="$(<"$_AAJ_EDITOR")"
  [[ -n "$editor" ]] && rm -rf "$(dirname "$editor")"
  rm -f "$_AAJ_WS" "$_AAJ_EDITOR" "${GBX_CONFIG_DIR}/active-project"
}

test_AAJ-plugin-add_stores_fragment() {
  local ws pid editor pdir
  ws="$(<"$_AAJ_WS")"; pid="$( _pid_hash "$ws" )"; editor="$(<"$_AAJ_EDITOR")"
  pdir="${GBX_CONFIG_DIR}/state/projects/${pid}/plugins"

  EDITOR="$editor" VISUAL="" run "${REPO_ROOT}/bin/gbx" -p "$pid" plugin add
  assert_success
  assert_output --partial "Run \`gbx rebuild\`"
  # Exactly one non-hidden file landed in the plugins dir.
  local count
  count="$(find "$pdir" -maxdepth 1 -type f ! -name '.*' | wc -l | tr -d ' ')"
  [[ "$count" == "1" ]] || fail "expected 1 stored plugin, found $count"
}

test_AAJ-plugin-ls_shows_description() {
  local pid
  pid="$( _pid_hash "$(<"$_AAJ_WS")" )"
  run "${REPO_ROOT}/bin/gbx" -p "$pid" plugin ls
  assert_success
  assert_output --partial "integration test plugin"
}

test_AAJ-plugin-rebuild_builds_derived_image() {
  local pid
  pid="$( _pid_hash "$(<"$_AAJ_WS")" )"
  run "${REPO_ROOT}/bin/gbx" -p "$pid" rebuild
  assert_success
  docker image inspect "glovebox-agent-${pid}:local" >/dev/null 2>&1 \
    || fail "derived image glovebox-agent-${pid}:local not found after rebuild"
}

test_AAJ-plugin-rm_then_rebuild_reverts_to_base() {
  local pid pdir frag id
  pid="$( _pid_hash "$(<"$_AAJ_WS")" )"
  pdir="${GBX_CONFIG_DIR}/state/projects/${pid}/plugins"
  frag="$(find "$pdir" -maxdepth 1 -type f ! -name '.*' | head -n1)"
  [[ -n "$frag" ]] || fail "no stored fragment to remove in $pdir"
  id="$(basename "$frag")"

  run "${REPO_ROOT}/bin/gbx" -p "$pid" plugin rm "$id" -y
  assert_success
  run "${REPO_ROOT}/bin/gbx" -p "$pid" rebuild
  assert_success
  # Derived image should be gone again.
  if docker image inspect "glovebox-agent-${pid}:local" >/dev/null 2>&1; then
    fail "derived image should have been removed after the last plugin was rm'd"
  fi
}

TESTS=(
  test_AAJ-plugin-add_stores_fragment
  test_AAJ-plugin-ls_shows_description
  test_AAJ-plugin-rebuild_builds_derived_image
  test_AAJ-plugin-rm_then_rebuild_reverts_to_base
)
