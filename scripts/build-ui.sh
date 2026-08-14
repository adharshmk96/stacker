#!/usr/bin/env bash
#
# Builds the Nuxt UI and drops it where the Go binary embeds it from.
#
# The UI is a static SPA (`ssr: false`), so `nuxt generate` writes a plain tree
# of files to .output/public — that tree becomes internal/web/dist, which
# //go:embed pulls into the binary on the next `go build`.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="$ROOT/stacker-ui"
OUT_DIR="$UI_DIR/.output/public"
DIST_DIR="$ROOT/stacker-server/internal/web/dist"

if ! command -v bun >/dev/null 2>&1; then
  echo "error: bun is required to build the UI" >&2
  exit 1
fi

echo "==> installing UI dependencies"
bun install --cwd "$UI_DIR" --frozen-lockfile

echo "==> building UI"
bun run --cwd "$UI_DIR" generate

if [ ! -f "$OUT_DIR/index.html" ]; then
  echo "error: expected $OUT_DIR/index.html — did the Nuxt build change output?" >&2
  exit 1
fi

echo "==> staging build into $DIST_DIR"
# Replace wholesale: a stale asset left behind would still be embedded, and the
# hashed filenames mean nothing would ever overwrite it.
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
cp -R "$OUT_DIR"/. "$DIST_DIR"/

echo "==> done — run 'task build' to bake it into the binary"
