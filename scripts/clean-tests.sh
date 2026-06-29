#!/usr/bin/env bash

# Best-effort sweep of test residue: labeled containers, orphan agents, test
# images, dangling images/volumes, empty stack networks, and .test-config dirs.
# Every step is guarded so one failure never blocks the rest.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

command -v docker >/dev/null 2>&1 || { echo 'docker not on PATH; nothing to clean'; exit 0; }

echo '==> labeled test containers (io.glovebox.test=1)'
docker ps -aq --filter label=io.glovebox.test=1 | xargs -r docker rm -f >/dev/null 2>&1 || true

echo '==> legacy workspace-orphan agents (workspace under TMPDIR)'
tmp_root="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P)"
for c in $(docker ps -a --filter name=glovebox-agent- --format '{{.Names}}' 2>/dev/null); do
  ws="$(docker inspect "$c" --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}' 2>/dev/null)"
  case "$ws" in
    "$tmp_root"/*|/tmp/*|/private/tmp/*) docker rm -f "$c" >/dev/null 2>&1 || true ;;
  esac
done

echo '==> test-only agent images (glovebox-agent-test-*)'
docker images --filter reference='glovebox-agent-test-*' -q 2>/dev/null | xargs -r docker rmi -f >/dev/null 2>&1 || true

echo '==> dangling images (orphan layers from previous rebuilds)'
docker image prune -f >/dev/null 2>&1 || true

echo '==> empty glovebox-stack-* networks'
for n in $(docker network ls --format '{{.Name}}' 2>/dev/null | grep '^glovebox-stack-' || true); do
  count="$(docker network inspect "$n" --format '{{len .Containers}}' 2>/dev/null)"
  if [ "$count" = "0" ]; then docker network rm "$n" >/dev/null 2>&1 || true; fi
done

echo '==> dangling volumes'
docker volume prune -f >/dev/null 2>&1 || true

echo '==> .test-config* dirs'
rm -rf .test-config .test-config.w*

echo 'Test residue cleaned.'
