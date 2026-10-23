#!/usr/bin/env bash
# Developer convenience dispatcher. Release CI calls the target scripts
# directly so every GUI is built and tested on its native operating system.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

case "$(uname -s)" in
  Linux*)
    scripts/build-windows.sh
    scripts/build-linux.sh
    ;;
  Darwin*)
    scripts/build-windows.sh
    scripts/build-macos.sh
    ;;
  MINGW*|MSYS*|CYGWIN*)
    scripts/build-windows.sh
    ;;
  *)
    echo "unsupported build host: $(uname -s)" >&2
    exit 1
    ;;
esac
