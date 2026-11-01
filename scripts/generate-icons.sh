#!/bin/bash
# Rebuild the macOS icon from resources/icon.svg. Requires macOS and Node.js.
set -euo pipefail
cd "$(dirname "$0")/.."
icon_tools=$(mktemp -d)
trap 'rm -rf "$icon_tools"' EXIT
npm install --prefix "$icon_tools" --no-audit --no-fund --silent @resvg/resvg-js@2.6.2
ICON_TOOLS_DIR="$icon_tools" node <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { Resvg } = require(path.join(process.env.ICON_TOOLS_DIR, 'node_modules/@resvg/resvg-js'));
const source = fs.readFileSync('resources/icon.svg', 'utf8');
const viewBox = source.match(/viewBox="([^"]+)"/);
if (!viewBox) throw new Error('The source SVG must have a viewBox');
const [x, y, width, height] = viewBox[1].split(/[\s,]+/).map(Number);
if (x !== 0 || y !== 0 || width !== height) throw new Error('Expected a square SVG with a zero-origin viewBox');
// Preserve the source colors, including its background, inside the macOS mask.
const artwork = source.replace(/^[\s\S]*?<svg\b[^>]*>/, '').replace(/<\/svg>\s*$/, '');
// A continuous corner: three cubic segments per corner, with smooth shoulders.
const radius = 185;
const corner = [
  [[1.088, 0], [.869, 0], [.632, .074]],
  [[.372, .169], [.169, .372], [.074, .632]],
  [[0, .869], [0, 1.088], [0, 1.528]],
];
let shell = `M ${100 + 1.528 * radius} 100`;
const transforms = [
  ([a, b]) => [924 - a * radius, 100 + b * radius],
  ([a, b]) => [924 - b * radius, 924 - a * radius],
  ([a, b]) => [100 + a * radius, 924 - b * radius],
  ([a, b]) => [100 + b * radius, 100 + a * radius],
];
for (const transform of transforms) {
  shell += ` L ${transform([1.528, 0]).join(' ')}`;
  for (const segment of corner) shell += ' C ' + segment.map(transform).flat().join(' ');
}
shell += ' Z';
const scale = 824 / width;
const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
<defs><clipPath id="artwork"><path d="${shell}"/></clipPath><filter id="shadow" x="-20%" y="-20%" width="140%" height="150%"><feGaussianBlur stdDeviation="8"/></filter></defs>
<path d="${shell}" transform="translate(0 8)" fill="#000" opacity=".14" filter="url(#shadow)"/>
<g clip-path="url(#artwork)"><g transform="translate(100 100) scale(${scale})">${artwork}</g></g>
</svg>\n`;
fs.writeFileSync('macos/icon.svg', svg);
const iconset = path.join(process.env.ICON_TOOLS_DIR, 'MDFlick.iconset');
fs.mkdirSync(iconset);
for (const size of [16, 32, 128, 256, 512]) {
  for (const density of [1, 2]) {
    const pixels = size * density;
    const png = new Resvg(svg, { fitTo: { mode: 'width', value: pixels } }).render().asPng();
    fs.writeFileSync(path.join(iconset, `icon_${size}x${size}${density === 2 ? '@2x' : ''}.png`), png);
    if (pixels === 1024) fs.writeFileSync('resources/icon.png', png);
  }
}
execFileSync('iconutil', ['-c', 'icns', iconset, '-o', 'macos/icon.icns']);
console.log('Generated resources/icon.png, macos/icon.svg, and macos/icon.icns');
NODE
