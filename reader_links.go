package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/document"
)

func localDocumentIcon(dest string, size float32, color ui.Color) (markdown.InlineObject, bool) {
	if localReaderPath(dest) == "" {
		return markdown.InlineObject{}, false
	}
	return markdown.InlineObject{
		Width: size * 1.04, Height: size * .72, Depth: size * .08,
		Draw: func(p *ui.Painter, r ui.Rect) {
			// Folded corner and two text lines, scaled with the link label.
			x, y := r.X+size*.08, r.Y+size*.06
			w, h, fold := size*.55, size*.68, size*.18
			stroke := max(1, size*.065)
			p.StrokePolyline([][2]float32{
				{x, y}, {x + w - fold, y}, {x + w, y + fold},
				{x + w, y + h}, {x, y + h}, {x, y},
			}, stroke, color)
			p.StrokePolyline([][2]float32{
				{x + w - fold, y}, {x + w - fold, y + fold}, {x + w, y + fold},
			}, stroke, color)
			for _, line := range []float32{.52, .74} {
				lineY := y + h*line
				p.StrokePolyline([][2]float32{{x + w*.24, lineY}, {x + w*.76, lineY}}, stroke, color)
			}
		},
	}, true
}

func localReaderPath(dest string) string {
	path := document.LocalFilePath(dest)
	if document.IsMarkdownPath(path) || isTextPreviewPath(path) {
		return path
	}
	return ""
}

func nativeReaderLink(dest string) document.LinkAction {
	if localReaderPath(dest) != "" {
		return document.OpenLocalDocument
	}
	return document.NativePolicy().Link(dest)
}
