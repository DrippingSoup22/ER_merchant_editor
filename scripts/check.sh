#!/usr/bin/env bash
# vet and test, building nothing. Pass packages to scope it:
#
#   scripts/check.sh ./internal/character/  # flags and bell bearings
#   scripts/check.sh ./internal/catalog/    # reads the fixture save
#   scripts/check.sh                        # everything
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." # repo root

pkgs=("$@")
[ ${#pkgs[@]} -eq 0 ] && pkgs=(./...)

echo "==> go vet ${pkgs[*]}"
go vet "${pkgs[@]}"
echo "==> go test ${pkgs[*]}"
go test "${pkgs[@]}"
echo "OK"
