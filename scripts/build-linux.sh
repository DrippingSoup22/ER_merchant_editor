#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ "$(go env GOOS)" != linux ]]; then
  echo "the Linux GUI must be built on Linux" >&2
  exit 1
fi

arch="$(go env GOARCH)"
version="${VERSION:-0.0.0}"
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  version="0.0.0"
fi
package="ER-Merchant-Editor-linux-$arch-$version"
out="dist/linux-$arch"
bundle="$out/$package"
archive="dist/$package.zip"
mkdir -p "$bundle"

echo "building ER Merchant Editor for linux/$arch"
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$bundle/ERMerchantEditor" ./cmd/ermerchanteditor
cp packaging/linux/io.github.daniele.ERMerchantEditor.desktop "$bundle/"
cp packaging/windows/winres/icon.png "$bundle/ERMerchantEditor.png"
cp packaging/linux/README.txt "$bundle/"

rm -f "$archive"
go run ./tools/packagezip -root "$bundle" -out "$archive"
