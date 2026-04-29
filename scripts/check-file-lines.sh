#!/usr/bin/env bash
set -euo pipefail

LIMIT="${LIMIT:-500}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT"

failed=0
while IFS= read -r -d '' file; do
  lines="$(wc -l < "$file" | tr -d ' ')"
  if [ "$lines" -gt "$LIMIT" ]; then
    printf '%s %s\n' "$lines" "$file"
    failed=1
  fi
done < <(
  find . \
    -path ./.git -prune -o \
    -path ./dist -prune -o \
    -path ./vendor -prune -o \
    -type f \( \
      -name '*.go' -o \
      -name '*.sh' -o \
      -name '*.py' -o \
      -name '*.js' -o \
      -name '*.ts' -o \
      -name '*.tsx' -o \
      -name '*.jsx' \
    \) -print0
)

if [ "$failed" -ne 0 ]; then
  printf 'code files above %s lines; split them by responsibility\n' "$LIMIT" >&2
  exit 1
fi

printf 'all code files are at or below %s lines\n' "$LIMIT"
