#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ "$(go env GOOS)" != linux ]]; then
  echo "the Linux GUI must be built on Linux" >&2
  exit 1
fi

arch="$(go env GOARCH)"
out="dist/linux-$arch"
bundle="$out/ERMerchantEditor-linux-$arch"
mkdir -p "$bundle"

echo "building ER Merchant Editor for linux/$arch"
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$bundle/ERMerchantEditor" ./cmd/ermerchanteditor
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
  -o "$bundle/shopwrite" ./cmd/shopwrite
cp packaging/linux/io.github.daniele.ERMerchantEditor.desktop "$bundle/"
cp packaging/windows/winres/icon.png "$bundle/ERMerchantEditor.png"
cp packaging/linux/README.txt "$bundle/"

tar -C "$out" -czf "dist/ERMerchantEditor-linux-$arch.tar.gz" "$(basename "$bundle")"
