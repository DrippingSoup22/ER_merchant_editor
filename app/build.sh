#!/usr/bin/env bash
# Builds everything we ship, from WSL/Linux/CI alike -- Go builds, no Python:
#
#   app/dist/ERMerchantEditor-windows-amd64.exe
#                                          the Windows GUI (icons + data
#                                          embedded; ~90MB, single file)
#   app/dist/ERMerchantEditor-linux-amd64  the native Linux GUI (same embedded
#                                          data; needs the usual desktop libs)
#   app/dist/io.github.daniele.ERMerchantEditor.desktop
#   app/dist/install-linux-desktop.sh       optional Linux menu/icon installer
#   app/dist/shopwrite/<goos>-<goarch>/    the shopwrite CLI, 2 targets,
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

echo "building ERMerchantEditor-windows-amd64.exe (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-H windowsgui -s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$DIST/ERMerchantEditor-windows-amd64.exe" ./app/cmd/editor

echo "building ERMerchantEditor-linux-amd64 (linux/amd64)..."
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$DIST/ERMerchantEditor-linux-amd64" ./app/cmd/editor

# A bare executable cannot carry an icon on Linux: desktop shells associate
# an icon with the Wayland/X11 app ID through a .desktop entry. Ship the
# source icon and a tiny opt-in installer next to the portable binary.
cp app/winres/icon.png "$DIST/io.github.daniele.ERMerchantEditor.png"
cp app/linux/io.github.daniele.ERMerchantEditor.desktop "$DIST/"
cp app/linux/install-linux-desktop.sh "$DIST/"
chmod +x "$DIST/install-linux-desktop.sh"

targets=(
  "linux amd64"
  "windows amd64"
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
ls -lh "$DIST/ERMerchantEditor-windows-amd64.exe" "$DIST/ERMerchantEditor-linux-amd64"
