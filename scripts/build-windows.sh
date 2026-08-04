#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

arch="amd64"
version="${VERSION:-0.0.0}"
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  version="0.0.0"
fi
package="ER-Merchant-Editor-windows-$arch-$version"
out="dist/windows-$arch"
bundle="$out/$package"
archive="dist/$package.zip"
mkdir -p "$bundle"

echo "building ER Merchant Editor for windows/$arch"
CGO_ENABLED=0 GOOS=windows GOARCH="$arch" \
  go build -trimpath -ldflags "-H windowsgui -s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$bundle/ERMerchantEditor.exe" ./cmd/ermerchanteditor

rm -f "$archive"
go run ./tools/packagezip -root "$bundle" -out "$archive"
