#!/bin/bash
# Rebuild the standard vendor tree and restore MDFlick's narrowly scoped patch.
set -euo pipefail
cd "$(dirname "$0")/.."
version=$(go list -mod=mod -m -f '{{.Version}}' github.com/egoist/mygo)
if [[ "$version" != v0.10.0 ]]; then
  echo "Review patches/mygo-v0.10.0.patch before upgrading MyGo ($version)." >&2
  exit 1
fi
tool_version=$(go -C tools list -m -f '{{.Version}}' github.com/egoist/mygo)
if [[ "$tool_version" != "$version" ]]; then
  echo "MyGo CLI ($tool_version) in tools/go.mod must match the app ($version)." >&2
  exit 1
fi
go mod vendor
git apply --check patches/mygo-v0.10.0.patch
git apply patches/mygo-v0.10.0.patch
