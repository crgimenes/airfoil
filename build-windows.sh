#!/usr/bin/env bash

set -euo pipefail

# build-windows.sh builds the official Windows release binaries (amd64, arm64)
# and refuses to produce an artifact whose dependencies are not the pinned,
# hash-verified module versions.
#
# It exists because of v0.1.9: the release ran from the repo directory, the
# local go.work silently resolved glaze/minigui/native to the sibling
# checkouts, and the published binaries report those modules as (devel), with
# untagged code inside and no way to reproduce the build from the tag. Every
# official build here runs with GOWORK=off, and every artifact is inspected
# with `go version -m` before it is accepted.
#
# Usage:
#   build-windows.sh [outdir]        build the .exe into outdir (default dist)
#   build-windows.sh verify FILE...  only run the dependency gate on FILEs
#
# release.sh calls both forms: the first for the Windows targets, the second
# for the macOS universal binary it builds itself.

# verify_pinned_deps fails when a binary's build info reports a (devel) or
# replaced dependency, which means a workspace or replace directive leaked into
# what should be a reproducible build from pinned versions.
verify_pinned_deps() {
  local bin="$1"
  local info
  info=$(go version -m "$bin")
  if echo "$info" | grep -qE '\(devel\)'; then
    echo "error: $bin was built with workspace-resolved modules:" >&2
    echo "$info" | grep -E '\(devel\)' >&2
    echo "hint: official builds must run with GOWORK=off." >&2
    return 1
  fi
  if echo "$info" | grep -qE '^\t=>'; then
    echo "error: $bin was built with replace directives:" >&2
    echo "$info" | grep -E '^\t=>' >&2
    return 1
  fi
  echo "  verified: $(basename "$bin") uses pinned module versions"
}

if [ "${1:-}" = "verify" ]; then
  shift
  if [ $# -lt 1 ]; then
    echo "usage: build-windows.sh verify FILE..." >&2
    exit 1
  fi
  for f in "$@"; do
    verify_pinned_deps "$f"
  done
  exit 0
fi

OUT_DIR="${1:-dist}"
BINARY_NAME=$(basename "$(pwd)")
mkdir -p "$OUT_DIR"

# 64-bit only: old platforms are not supported here, and a 32-bit target has
# to be built, signed, uploaded and answered for on every release forever.
for arch in amd64 arm64; do
  out="$OUT_DIR/${BINARY_NAME}-windows-${arch}.exe"
  echo "Building windows/${arch}: $(basename "$out")"
  env GOWORK=off GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o "$out" .
  verify_pinned_deps "$out"
done
