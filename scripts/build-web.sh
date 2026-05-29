#!/bin/sh
set -eu

# Build the React web app and stage its output into internal/serve/web/ for
# Go's //go:embed directive.
#
# Requires NPM_AUTH_TOKEN to be exported (a GitHub PAT with read:packages
# scope, or secrets.GITHUB_TOKEN in CI). The token is referenced from
# web/.npmrc; copy web/.npmrc.example -> web/.npmrc once per checkout.

repoRoot="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repoRoot"

if [ ! -f web/.npmrc ]; then
    cp web/.npmrc.example web/.npmrc
fi

(cd web && ./bun install)
(cd web && ./bun run build)

embedDir="internal/serve/web"
find "$embedDir" -mindepth 1 ! -name '.gitkeep' -delete
cp -R web/dist/. "$embedDir/"
