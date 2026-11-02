//go:build darwin

package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Core Text shapes and selects fallback fonts; Core Graphics supplies exact
// glyph curves. The result remains SVG paths, independent of display scale.
// No AppKit views, main-thread dispatch, browser, or subprocess is involved.
var svgTextNative struct {
	once              sync.Once
	err               error
	release           func(uintptr)
	stringCreate      func(uintptr, *byte, int, uint32, bool) uintptr
	dictCreate        func(uintptr, *uintptr, *uintptr, int, uintptr, uintptr) uintptr
	dictValue         func(uintptr, uintptr) uintptr
	attributedCreate  func(uintptr, uintptr, uintptr) uintptr
	fontCreate        func(uintptr, float64, uintptr) uintptr
	fontTraits        func(uintptr, float64, uintptr, uint32, uint32) uintptr
	lineCreate        func(uintptr) uintptr
	lineBounds        func(uintptr, *float64, *float64, *float64) float64
	lineRuns          func(uintptr) uintptr
	arrayCount        func(uintptr) int
	arrayValue        func(uintptr, int) uintptr
	runCount          func(uintptr) int
	runAttrs          func(uintptr) uintptr
	runGlyphs         func(uintptr, int, int, *uint16)
	runPositions      func(uintptr, int, int, *svgCTPoint)
	fontPath          func(uintptr, uint16, uintptr) uintptr
	pathApply         func(uintptr, uintptr, uintptr)
	fontKey, callback uintptr
	// Only the synchronous CGPathApply callback reads these, under svgTextGate.
	output    *strings.Builder
	x, y      float64
	outputErr error
}
var svgTextGate = make(chan struct{}, 1)

type svgCTPoint struct{ x, y float64 }
type svgCTPathElement struct {
	kind   int32
	points *svgCTPoint
}

func initSVGTextNative() {
	api := &svgTextNative
	open := func(path string) uintptr {
		handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil && api.err == nil {
			api.err = err
		}
		return handle
	}
	cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	ct := open("/System/Library/Frameworks/CoreText.framework/CoreText")
	cg := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	if api.err != nil {
		return
	}
	symbol := func(handle uintptr, name string) uintptr {
		address, err := purego.Dlsym(handle, name)
		if err != nil && api.err == nil {
			api.err = err
		}
		return address
	}
	bind := func(handle uintptr, name string, dest any) {
		if address := symbol(handle, name); address != 0 {
			purego.RegisterFunc(dest, address)
		}
	}
	bind(cf, "CFRelease", &api.release)
	bind(cf, "CFStringCreateWithBytes", &api.stringCreate)
	bind(cf, "CFDictionaryCreate", &api.dictCreate)
	bind(cf, "CFDictionaryGetValue", &api.dictValue)
	bind(cf, "CFAttributedStringCreate", &api.attributedCreate)
	bind(cf, "CFArrayGetCount", &api.arrayCount)
	bind(cf, "CFArrayGetValueAtIndex", &api.arrayValue)
	bind(ct, "CTFontCreateWithName", &api.fontCreate)
	bind(ct, "CTFontCreateCopyWithSymbolicTraits", &api.fontTraits)
	bind(ct, "CTLineCreateWithAttributedString", &api.lineCreate)
	bind(ct, "CTLineGetTypographicBounds", &api.lineBounds)
	bind(ct, "CTLineGetGlyphRuns", &api.lineRuns)
	bind(ct, "CTRunGetGlyphCount", &api.runCount)
	bind(ct, "CTRunGetAttributes", &api.runAttrs)
	// CFRange is two machine integers, passed in consecutive argument registers
	// on both supported macOS ABIs (arm64 and amd64), as in MyGo's bindings.
	bind(ct, "CTRunGetGlyphs", &api.runGlyphs)
	bind(ct, "CTRunGetPositions", &api.runPositions)
	bind(ct, "CTFontCreatePathForGlyph", &api.fontPath)
	bind(cg, "CGPathApply", &api.pathApply)
	if address := symbol(ct, "kCTFontAttributeName"); address != 0 {
		api.fontKey = **(**uintptr)(unsafe.Pointer(&address))
	}
	if api.err != nil {
		return
	}
	api.callback = purego.NewCallback(func(_ uintptr, element *svgCTPathElement) {
		if api.outputErr != nil {
			return
		}
		count := 0
		command := ""
		switch element.kind {
		case 0:
			command = "M"
			count = 1
		case 1:
			command = "L"
			count = 1
		case 2:
			command = "Q"
			count = 2
		case 3:
			command = "C"
			count = 3
		case 4:
			command = "Z"
		}
		api.output.WriteString(command)
		for _, p := range unsafe.Slice(element.points, count) {
			api.output.WriteByte(' ')
			api.output.WriteString(svgNumber(p.x + api.x))
			api.output.WriteByte(' ')
			api.output.WriteString(svgNumber(-p.y - api.y))
		}
		if api.output.Len() > maxImageBytes {
			api.outputErr = fmt.Errorf("SVG 文字轮廓过大")
		}
	})
}

func shapeSVGText(ctx context.Context, text string, style svgTextStyle) (svgTextOutline, error) {
	if err := ctx.Err(); err != nil {
		return svgTextOutline{}, err
	}
	if text == "" {
		return svgTextOutline{}, nil
	}
	select {
	case svgTextGate <- struct{}{}:
	case <-ctx.Done():
		return svgTextOutline{}, ctx.Err()
	}
	defer func() { <-svgTextGate }()
	api := &svgTextNative
	api.once.Do(initSVGTextNative)
	if api.err != nil {
		return svgTextOutline{}, api.err
	}
	makeString := func(s string) uintptr {
		b := []byte(s)
		if len(b) == 0 {
			return 0
		}
		value := api.stringCreate(0, &b[0], len(b), 0x08000100, false)
		runtime.KeepAlive(b)
		return value
	}
	family := style.family
	if family == "" || family == "sans-serif" || family == "system-ui" {
		family = "Helvetica"
	}
	if family == "serif" {
		family = "Times New Roman"
	}
	if family == "monospace" {
		family = "Menlo"
	}
	name := makeString(family)
	if name == 0 {
		return svgTextOutline{}, fmt.Errorf("无法创建 SVG 字体名称")
	}
	defer api.release(name)
	font := api.fontCreate(name, style.size, 0)
	if font == 0 {
		return svgTextOutline{}, fmt.Errorf("无法创建 SVG 字体")
	}
	traits := uint32(0)
	if style.bold {
		traits |= 2
	}
	if style.italic {
		traits |= 1
	}
	if traits != 0 {
		if styled := api.fontTraits(font, style.size, 0, traits, traits); styled != 0 {
			api.release(font)
			font = styled
		}
	}
	defer api.release(font)
	value := makeString(text)
	if value == 0 {
		return svgTextOutline{}, fmt.Errorf("无法创建 SVG 文字")
	}
	defer api.release(value)
	key := api.fontKey
	attrs := api.dictCreate(0, &key, &font, 1, 0, 0)
	if attrs == 0 {
		return svgTextOutline{}, fmt.Errorf("无法创建 SVG 字体属性")
	}
	defer api.release(attrs)
	attributed := api.attributedCreate(0, value, attrs)
	if attributed == 0 {
		return svgTextOutline{}, fmt.Errorf("无法创建 SVG 排版文字")
	}
	defer api.release(attributed)
	line := api.lineCreate(attributed)
	if line == 0 {
		return svgTextOutline{}, fmt.Errorf("无法排版 SVG 文字")
	}
	defer api.release(line)
	var result svgTextOutline
	result.width = api.lineBounds(line, &result.ascent, &result.descent, nil)
	runs := api.lineRuns(line)
	var path strings.Builder
	api.output, api.outputErr = &path, nil
	defer func() { api.output = nil }()
	glyphIndex := 0
	for i, n := 0, api.arrayCount(runs); i < n; i++ {
		run := api.arrayValue(runs, i)
		count := api.runCount(run)
		if count < 0 || count > maxSVGTextBytes-glyphIndex {
			return svgTextOutline{}, fmt.Errorf("SVG 字形数量过多")
		}
		glyphs := make([]uint16, count)
		positions := make([]svgCTPoint, count)
		if count == 0 {
			continue
		}
		api.runGlyphs(run, 0, count, &glyphs[0])
		api.runPositions(run, 0, count, &positions[0])
		runFont := api.dictValue(api.runAttrs(run), api.fontKey)
		for j, glyph := range glyphs {
			if err := ctx.Err(); err != nil {
				return svgTextOutline{}, err
			}
			outline := api.fontPath(runFont, glyph, 0)
			api.x, api.y = positions[j].x+float64(glyphIndex)*style.spacing, positions[j].y
			glyphIndex++
			if outline == 0 {
				continue
			} // Spaces and color-only glyphs have no outline.
			api.pathApply(outline, 0, api.callback)
			api.release(outline)
			if api.outputErr != nil {
				return svgTextOutline{}, api.outputErr
			}
		}
	}
	result.width += float64(max(0, glyphIndex-1)) * style.spacing
	result.path = path.String()
	return result, nil
}
