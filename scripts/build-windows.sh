#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

out="dist/windows-amd64"
mkdir -p "$out"

echo "building ER Merchant Editor for windows/amd64"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-H windowsgui -s -w -X gioui.org/app.ID=io.github.daniele.ERMerchantEditor" \
  -o "$out/ERMerchantEditor.exe" ./cmd/ermerchanteditor

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w" \
  -o "$out/shopwrite.exe" ./cmd/shopwrite
