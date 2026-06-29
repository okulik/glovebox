#!/usr/bin/env bash

# Tag a release. By default re-tags the current version.txt without bumping
# (so a hand-edited version.txt can be tagged directly). Pass BUMP=patch,
# BUMP=minor, or BUMP=major to bump first, commit the bump, then tag.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

bump="${BUMP:-}"

if [ -n "$(git status --porcelain)" ]; then
  echo "Working tree not clean; commit or stash first." >&2
  exit 1
fi

current="$(tr -d '[:space:]' < version.txt)"

if [ -z "$bump" ]; then
  new="$current"
  echo "Tagging current version v$new (no bump)"
else
  IFS=. read -r major minor patch <<< "$current"
  case "$bump" in
    major) new="$((major + 1)).0.0" ;;
    minor) new="$major.$((minor + 1)).0" ;;
    patch) new="$major.$minor.$((patch + 1))" ;;
    *) echo "BUMP must be one of: patch, minor, major (or unset to re-tag current)" >&2; exit 1 ;;
  esac
  echo "$current → $new ($bump bump)"
  echo "$new" > version.txt
  git add version.txt
  git commit -m "chore: release v$new"
fi

if git rev-parse --verify "v$new" >/dev/null 2>&1; then
  echo "Tag v$new already exists locally; pick a new version with BUMP=…" >&2
  exit 1
fi

git tag -a "v$new" -m "Release v$new"

echo
echo "Created tag v$new locally. To publish:"
echo "  1) git push && git push origin v$new"
echo "  2) curl -sL https://github.com/okulik/glovebox/archive/refs/tags/v$new.tar.gz | shasum -a 256"
echo "  3) In Formula/glovebox.rb uncomment url/sha256/version and fill them in for v$new"
echo "  4) git add Formula/glovebox.rb && git commit -m 'brew: pin v$new' && git push"
