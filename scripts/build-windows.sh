#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
arch="${1:-amd64}"
case "$arch" in amd64|arm64) ;; *) echo 'Usage: bash scripts/build-windows.sh [amd64|arm64]' >&2; exit 1 ;; esac
if ! command -v makensis >/dev/null 2>&1; then
  echo 'NSIS is required: brew install makensis (macOS), or apt install nsis (Linux).' >&2
  exit 1
fi
# Build into a fresh directory: an old setup must never satisfy validation.
staging=$(mktemp -d "$PWD/.windows-build.XXXXXX")
trap 'rm -rf "$staging"' EXIT
go tool mygo build -platform "windows/$arch" -o "$staging"
output="$staging/windows-$arch"
shopt -s nullglob
installers=("$output"/*-setup-*-"$arch".exe)
if [[ ${#installers[@]} -ne 1 ]]; then
  echo "Expected one Windows installer, found ${#installers[@]}." >&2
  exit 1
fi
mkdir -p "build/windows-$arch"
cp -R "$output/." "build/windows-$arch/"
echo "Built build/windows-$arch/$(basename "${installers[0]}")"
