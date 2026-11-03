// Package svg embeds the browser rasterizer used for all document SVG images.
package svg

import _ "embed"

// RasterizerScript is trusted code for the application's existing browser pool.
// Source SVGs are supplied separately as base64 data, never as JavaScript.
//
//go:embed rasterize.js
var RasterizerScript string
