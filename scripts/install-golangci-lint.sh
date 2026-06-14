#!/usr/bin/env bash
# Idempotently install golangci-lint into ./bin/ at the pinned version.
# Re-running is a no-op once the right version is already present.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

WANT_VERSION="v2.12.2"

if [[ -x bin/golangci-lint ]]; then
  have="$(bin/golangci-lint --version 2>/dev/null | awk '{print $4}' | head -1)"
  if [[ "$have" == "${WANT_VERSION#v}" ]]; then
    exit 0
  fi
fi

mkdir -p bin
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ./bin "$WANT_VERSION"
