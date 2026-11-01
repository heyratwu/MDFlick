#!/bin/bash
# Build the host with MyGo, embed the extension, then sign inside out.
set -euo pipefail
cd "$(dirname "$0")/.."
arch="${1:-$(go env GOARCH)}"
case "$arch" in arm64) swift_arch=arm64 ;; amd64) swift_arch=x86_64 ;; *) echo 'Usage: bash scripts/build-macos.sh [arm64|amd64]' >&2; exit 1 ;; esac
identity="${SIGNING_IDENTITY:--}"
export GOCACHE="${GOCACHE:-$PWD/.mygo/go-cache}"
export CLANG_MODULE_CACHE_PATH="$PWD/.mygo/clang-cache"
go tool mygo build -platform "darwin/$arch" -skip-dmg -sign "$identity"
app="build/darwin-$arch/MDFlick.app"
if [[ ! -d "$app" ]]; then echo "Missing app: $app" >&2; exit 1; fi
extension="$app/Contents/PlugIns/MDFlickQuickLook.appex"
mkdir -p "$extension/Contents/MacOS"
renderer="build/darwin-$arch/mdpreview-renderer"
CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CGO_CFLAGS="-mmacosx-version-min=12.0" go build -buildmode=c-archive -trimpath -ldflags='-s -w' -o "$renderer.a" ./cmd/mdpreviewlib
# Remove the old subprocess renderer when rebuilding an existing bundle.
rm -f "$extension/Contents/MacOS/mdpreview"
cp macos/QuickLook/Info.plist "$extension/Contents/Info.plist"
xcrun swiftc -module-name MDFlickQuickLook -parse-as-library -emit-executable \
  -target "$swift_arch-apple-macosx12.0" -application-extension \
  -framework Cocoa -framework QuickLookUI -framework WebKit \
  -Xlinker -e -Xlinker _NSExtensionMain \
  -import-objc-header "$renderer.h" "$renderer.a" \
  macos/QuickLook/PreviewBridge.swift macos/QuickLook/PreviewViewController.swift -o "$extension/Contents/MacOS/MDFlickQuickLook"
codesign --force --options runtime --sign "$identity" --entitlements macos/QuickLook/entitlements.plist "$extension"
codesign --force --options runtime --sign "$identity" "$app"
codesign --verify --deep --strict "$app"
# Package only after the extension has been embedded and signed.
version=$(/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" "$app/Contents/Info.plist")
dmg="build/darwin-$arch/mdflick-$version-$arch.dmg"
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
ditto "$app" "$staging/MDFlick.app"
ln -s /Applications "$staging/Applications"
hdiutil create -volname MDFlick -srcfolder "$staging" -ov -format UDZO "$dmg"
echo "Built $app and $dmg"
