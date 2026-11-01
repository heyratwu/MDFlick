package main

import (
	"errors"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestMermaidPreviewStates(t *testing.T) {
	code := "flowchart LR\n A[中文] --> B[预览]\n"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/diagram.md", source: "```mermaid\n" + code + "```\n\n```go\nprintln(1)\n```"}, md: &markdown.Options{}, renderer: &diagramRenderer{}, diagrams: make(map[diagramKey]*diagramResult)}
	a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock}
	light := diagramKey{code, false}
	dark := diagramKey{code, true}
	// An existing pending entry must stay pending across repeated builds.
	a.diagrams[light] = nil
	tt := ui.NewTester(a.view, 480, 600)
	tt.Frame()
	if !tt.HasText("正在渲染 Mermaid 图表…") {
		t.Fatal(tt.Texts())
	}
	a.diagrams[light] = &diagramResult{image: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1000, 500))), width: 1000}
	tt.Frame()
	if tt.HasText("正在渲染 Mermaid 图表…") || !tt.HasText("Mermaid") {
		t.Fatal(tt.Texts())
	}
	a.diagrams[dark] = &diagramResult{err: errors.New("invalid diagram")}
	tt.SetDark(true)
	tt.Frame()
	if !tt.HasText("Mermaid 图表无法渲染，显示源码") {
		t.Fatal(tt.Texts())
	}
	// Disappear and return with the same options and cached results.
	a.loading = true
	tt.Frame()
	a.loading = false
	tt.Frame()
	tt.SetDark(false)
	tt.Frame()
	if !tt.HasText("Mermaid") {
		t.Fatal(tt.Texts())
	}
}
