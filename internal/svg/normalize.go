// Package svg prepares static SVG images for the native renderer without depending on MyGo.
package svg

import "context"

const maxOutputBytes = 16 << 20

// Normalize resolves root CSS variables and converts supported text to paths.
// Text shaping uses Core Text on macOS; other platforms reject text-bearing SVGs.
// Unrelated SVG markup is preserved. No image decoding or UI work happens here.
func Normalize(ctx context.Context, data []byte) ([]byte, error) {
	data, err := nativeSVGVariables(ctx, data)
	if err != nil {
		return nil, err
	}
	return nativeSVGText(ctx, data)
}
