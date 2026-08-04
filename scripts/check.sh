#!/usr/bin/env bash
# Fast post-edit sanity check: `go vet` + `go test`, native toolchain, NO
# build artifacts. This is the iteration-loop command; scripts/build.sh is
# only for producing shippable archives, not for checking that an edit compiles.
#
# SCOPE IT to the package(s) you touched -- fixture/editor tests are slow
# (AES-decrypt the fixture save; editor needs cgo/X11), so passing a package
# is the fast path:
#
#   scripts/check.sh ./internal/character/flags/ # <1s
#   scripts/check.sh ./internal/catalog/         # fixture crypto
#   scripts/check.sh ./internal/ui/gio/          # cgo/X11
#   scripts/check.sh                              # everything; pre-push
#
# CI (.github/workflows/release.yml) still runs the full vet+test+build on
# push, so this is a local convenience, not the source of truth.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." # repo root

pkgs=("$@")
[ ${#pkgs[@]} -eq 0 ] && pkgs=(./...)

echo "==> go vet ${pkgs[*]}"
go vet "${pkgs[@]}"
echo "==> go test ${pkgs[*]}"
go test "${pkgs[@]}"
echo "OK"
