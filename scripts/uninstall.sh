#!/usr/bin/env bash

# Remove every glovebox container, network, volume, the agent image, and the
# config dir. Set FORCE=1 to skip the confirmation prompt. Best-effort: each
# step is guarded so one failure never blocks the rest.

if [ "${FORCE:-}" != "1" ]; then
  printf 'This removes every glovebox container, the agent image, and %s. Continue? [y/N] ' "${GBX_CONFIG_DIR:-$HOME/.config/glovebox}"
  read -r ans
  [ "$ans" = "y" ] || { echo 'Aborted.'; exit 1; }
fi

docker container ls -a --filter name='^glovebox-' --format '{{.Names}}' |
  xargs -r docker rm -f >/dev/null 2>&1 || true
docker network ls --format '{{.Name}}' |
  grep '^glovebox' |
  xargs -r -I {} docker network rm {} >/dev/null 2>&1 || true
docker volume ls --format '{{.Name}}' |
  grep '^glovebox' |
  xargs -r docker volume rm >/dev/null 2>&1 || true
docker image rm glovebox-agent:local >/dev/null 2>&1 || true
rm -rf "${GBX_CONFIG_DIR:-$HOME/.config/glovebox}"
echo "Glovebox uninstalled."
