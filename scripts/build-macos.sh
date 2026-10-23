#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ "$(go env GOOS)" != darwin ]]; then
  echo "the macOS GUI must be built on macOS with Xcode installed" >&2
  exit 1
fi

arch="$(go env GOARCH)"
version="${VERSION:-0.0.0}"
version="${version#v}"
# Apple's bundle versions must be one to three dot-separated integers. Keep
# local/manual branch builds valid even when VERSION contains a branch name.
if [[ ! "$version" =~ ^[0-9]+(\.[0-9]+){0,2}$ ]]; then
  version="0.0.0"
fi
out="dist/macos-$arch"
app="$out/ER Merchant Editor.app"
contents="$app/Contents"
mkdir -p "$contents/MacOS" "$contents/Resources"

echo "building ER Merchant Editor for darwin/$arch"
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X gioui.org/app.ID=io.github.DrippingSoup22.ERMerchantEditor" \
  -o "$contents/MacOS/ERMerchantEditor" ./cmd/ermerchanteditor

sed "s/@VERSION@/$version/g" packaging/macos/Info.plist > "$contents/Info.plist"
iconset="$out/ERMerchantEditor.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" packaging/windows/winres/icon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" packaging/windows/winres/icon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$contents/Resources/ERMerchantEditor.icns"
rm -rf "$iconset"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  codesign --force --deep --options runtime --timestamp --sign "$MACOS_SIGN_IDENTITY" "$app"
else
  codesign --force --deep --sign - "$app"
fi

ditto -c -k --sequesterRsrc --keepParent "$app" "dist/ERMerchantEditor-macos-$arch.zip"
