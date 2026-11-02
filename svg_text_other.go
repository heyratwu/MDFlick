//go:build !darwin

package main

import (
	"context"
	"fmt"
)

func shapeSVGText(ctx context.Context, text string, style svgTextStyle) (svgTextOutline, error) {
	return svgTextOutline{}, fmt.Errorf("SVG 原生文字轮廓仅支持 macOS")
}
