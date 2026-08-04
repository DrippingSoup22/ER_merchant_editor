#!/usr/bin/env bash
# Builds everything we ship, from WSL/Linux/CI alike -- pure `go build`
# cross-compilation, no cgo, no Python, no target-OS toolchain:
#
#   app/dist/ERMerchantEditor.exe          the Windows GUI (icons + data
#                                          embedded; ~90MB, single file)
#   app/dist/shopwrite/<goos>-<goarch>/    the shopwrite CLI, 4 targets,
#                                          schema embedded, standalone
#
# The Windows resource object (exe icon, manifest, version info) is the
# committed app/cmd/editor/rsrc_windows_amd64.syso; regenerate it after
# changing app/winres/* with:
#   go tool go-winres make --in app/winres/winres.json --out app/cmd/editor/rsrc --arch amd64
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." # repo root

DIST=app/dist
rm -rf "$DIST"
mkdir -p "$DIST"

echo "building ERMerchantEditor.exe (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-H windowsgui -s -w" \
  -o "$DIST/ERMerchantEditor.exe" ./app/cmd/editor

targets=(
  "linux amd64"
  "windows amd64"
  "darwin amd64"
  "darwin arm64"
)
for t in "${targets[@]}"; do
  read -r goos goarch <<<"$t"
  out="$DIST/shopwrite/$goos-$goarch"
  mkdir -p "$out"
  bin="shopwrite"
  [ "$goos" = "windows" ] && bin="shopwrite.exe"
  echo "building shopwrite $goos/$goarch..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$out/$bin" ./app/cmd/shopwrite
done

echo "done:"
ls -lh "$DIST/ERMerchantEditor.exe"
