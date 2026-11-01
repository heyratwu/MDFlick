package main

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestMermaidPreviewStates(t *testing.T) {
	code := "flowchart LR\n A[中文] --> B[预览]\n"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/diagram.md", source: "```mermaid\n" + code + "```\n\n```go\nprintln(1)\n```"}, md: &markdown.Options{}, diagrams: newDiagramService(&fakeDiagramBackend{}, func(fn func()) { fn() }, &byteBudget{limit: 128 << 20})}
	a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock}
	light := diagramKey{code, false}
	dark := diagramKey{code, true}
	// An existing pending entry must stay pending across repeated builds.
	a.diagrams.entries[light] = nil
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 480, 600)
	tt.Frame()
	if !tt.HasText("正在渲染 Mermaid 图表…") {
		t.Fatal(tt.Texts())
	}
	a.diagrams.entries[light] = &diagramResult{image: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1000, 500))), width: 1000}
	tt.Frame()
	if tt.HasText("正在渲染 Mermaid 图表…") || !tt.HasText("Mermaid") {
		t.Fatal(tt.Texts())
	}
	a.diagrams.entries[dark] = &diagramResult{err: errors.New("invalid diagram")}
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

func TestMermaidDefersOffscreenWork(t *testing.T) {
	backend := &fakeDiagramBackend{}
	source := "```mermaid\nflowchart LR; A-->B\n```\n\n" + strings.Repeat("Paragraph.\n\n", 100) + "```mermaid\nflowchart LR; X-->Y\n```"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/visible.md", source: source}, md: &markdown.Options{}, diagrams: newDiagramService(backend, func(fn func()) { fn() }, &byteBudget{limit: 128 << 20})}
	a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 600, 500)
	for range 3 {
		tt.Frame()
	}
	if len(backend.callbacks) != 1 {
		t.Fatal("offscreen work admitted", len(backend.callbacks))
	}
	a.scroll.Y = a.scroll.MaxY
	for range 3 {
		tt.Frame()
	}
	if len(backend.callbacks) != 2 {
		t.Fatal("visible work never admitted", len(backend.callbacks), a.scroll)
	}
	a.diagrams.close()
}
