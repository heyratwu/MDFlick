package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *readerWindow) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView) {
	if b.Open || a.diagrams == nil {
		a.codeBlock(c, b)
		return
	}
	box := ui.Column(c)
	state := *ui.Local(box, diagramVisibilityKey{}, func() *diagramVisibility { return &diagramVisibility{} })
	box.Bind(&state.handle)
	bounds := state.handle.Bounds(c)
	bounds.Y += state.scrollY - a.scroll.Y
	state.scrollY = a.scroll.Y
	_, height := c.Size()
	visible := bounds.H > 0 && bounds.Y < height+200 && bounds.Y+bounds.H > readerTitleBarHeight-200
	if bounds.H == 0 {
		// One follow-up build after initial layout.
		c.AnimationFrame()
	}
	key := diagramKey{b.Code(), c.Theme().Dark}
	box.Children(func() { a.mermaidContent(c, b, key, visible) })
}

type diagramVisibilityKey struct{}
type diagramVisibility struct {
	handle  ui.Handle
	scrollY float32
}

func (a *readerWindow) mermaidContent(c *ui.Context, b *markdown.CodeBlockView, key diagramKey, visible bool) {
	// Completed images keep their geometry outside the viewport. Only unseen
	// uncached work waits; ordinary scrolling triggers its next admission.
	if _, exists := a.diagrams.entries[key]; !exists && !visible {
		previous := diagramKey{code: key.code, dark: !key.dark}
		if result := a.diagrams.entries[previous]; result != nil && result.err == nil {
			key = previous // Preserve offscreen geometry until the new theme is needed.
		} else {
			a.codeBlock(c, b)
			return
		}
	}
	result, admitted := a.diagrams.request(key)
	if !admitted {
		a.codeBlock(c, b)
		return
	}

	if result == nil {
		ui.Text(c, "正在渲染 Mermaid 图表…").TextColor(c.Theme().TextMuted)
		a.codeBlock(c, b)
		return
	}
	if result.err != nil {
		ui.Text(c, "Mermaid 图表无法渲染，显示源码").TextColor(c.Theme().TextMuted).Tooltip(fmt.Sprint(result.err))
		a.codeBlock(c, b)
		return
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Mermaid").TextColor(c.Theme().TextMuted)
		ui.Spacer(c)
		b.CopyButton(c)
	})
	ui.Image(c, result.image).Width(result.width).MaxWidthPercent(100).Label("Mermaid 图表").Role(ui.RoleImage)
}
