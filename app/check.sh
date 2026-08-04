#!/usr/bin/env bash
# Fast post-edit sanity check: `go vet` + `go test`, native toolchain, NO
# build artifacts. This is the iteration-loop command -- app/build.sh (24s,
# writes a 90MB exe + 4 CLI targets) is only for producing the shippable
# binary, not for checking that an edit compiles.
#
# SCOPE IT to the package(s) you touched -- fixture/editor tests are slow
# (AES-decrypt the fixture save; editor needs cgo/X11), so passing a package
# is the fast path:
#
#   app/check.sh ./app/charflags/    # <1s
#   app/check.sh ./app/catalog/      # ~13s (fixture crypto)
#   app/check.sh ./app/editor/       # ~23s (cgo/X11)
#   app/check.sh                     # everything (~40s) -- pre-push only
#
# CI (.github/workflows/release.yml) still runs the full vet+test+build on
# push, so this is a local convenience, not the source of truth.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." # repo root

pkgs=("$@")
[ ${#pkgs[@]} -eq 0 ] && pkgs=(./app/...)

echo "==> go vet ${pkgs[*]}"
go vet "${pkgs[@]}"
echo "==> go test ${pkgs[*]}"
go test "${pkgs[@]}"
echo "OK"
