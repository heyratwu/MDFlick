#!/bin/bash
# Exercise the real Swift/C/Go boundary without registering a Finder extension.
set -euo pipefail
cd "$(dirname "$0")/.."
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
CGO_ENABLED=1 CGO_CFLAGS="-mmacosx-version-min=12.0" go build -buildmode=c-archive -o "$staging/renderer.a" ./cmd/mdpreviewlib
xcrun swiftc -parse-as-library -import-objc-header "$staging/renderer.h" \
  macos/QuickLook/PreviewBridge.swift macos/QuickLookTests/BridgeTests.swift \
  "$staging/renderer.a" -o "$staging/bridge-tests"
"$staging/bridge-tests"
xcrun swiftc -typecheck -application-extension -import-objc-header "$staging/renderer.h" \
  macos/QuickLook/PreviewBridge.swift macos/QuickLook/PreviewViewController.swift
