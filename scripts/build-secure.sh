#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT/dist}"
BIN_NAME="${BIN_NAME:-bdy}"
VERSION="${VERSION:-secure}"
TARGET="$OUT_DIR/$BIN_NAME"

mkdir -p "$OUT_DIR"

build_cmd=(go build -trimpath -ldflags "-s -w -X baiduyunStorage/internal/cli.version=$VERSION" -o "$TARGET" ./cmd/bdy)

cd "$ROOT"

if command -v garble >/dev/null 2>&1; then
  GARBLE_BUILD=1 garble -literals -tiny build -trimpath -ldflags "-s -w -X baiduyunStorage/internal/cli.version=$VERSION" -o "$TARGET" ./cmd/bdy
else
  "${build_cmd[@]}"
fi

if command -v upx >/dev/null 2>&1; then
  upx --best --lzma "$TARGET" >/dev/null
fi

sha256sum "$TARGET" > "$TARGET.sha256"
chmod 0755 "$TARGET"

echo "built $TARGET"
echo "sha256 $(cut -d' ' -f1 "$TARGET.sha256")"
