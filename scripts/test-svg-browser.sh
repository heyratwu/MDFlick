#!/bin/bash
# Exercise the production SVG rasterizer in macOS WebKit with its real CSP.
set -euo pipefail
cd "$(dirname "$0")/.."
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
swiftc macos/SVGTests/BrowserTests.swift -o "$staging/svg-tests"
"$staging/svg-tests" internal/svg/rasterize.js internal/testfixtures/images
