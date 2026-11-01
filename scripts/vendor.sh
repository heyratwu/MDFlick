#!/bin/bash
# Rebuild the standard vendor tree and restore MDFlick's narrowly scoped patch.
set -euo pipefail
cd "$(dirname "$0")/.."
version=$(go list -mod=mod -m -f '{{.Version}}' github.com/egoist/mygo)
if [[ "$version" != v0.7.3 ]]; then
  echo "Review patches/mygo-v0.7.3.patch before upgrading MyGo ($version)." >&2
  exit 1
fi
go mod vendor
git apply --check patches/mygo-v0.7.3.patch
git apply patches/mygo-v0.7.3.patch
