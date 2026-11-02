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
	key := diagramKey{b.Code(), c.Theme().Dark}
	width, height := c.Size()
	frame := a.diagramFrame
	c.AfterPresent(func() {
		bounds := state.handle.Bounds(c)
		if distance, near := imageViewportDistance(bounds, width, height); near && frame != nil {
			if old, ok := frame[key]; !ok || distance < old {
				frame[key] = distance
			}
		}
	})
	bounds := state.handle.Bounds(c)
	visible := bounds.H > 0 && bounds.Y < height+200 && bounds.Y+bounds.H > readerTitleBarHeight-200
	box.Children(func() { a.mermaidContent(c, b, key, visible, state) })
}

type diagramVisibilityKey struct{}
type diagramVisibility struct {
	handle        ui.Handle
	width, height float32
}

func (a *readerWindow) mermaidContent(c *ui.Context, b *markdown.CodeBlockView, key diagramKey, visible bool, geometry *diagramVisibility) {
	// Completed images keep their geometry outside the viewport. Only unseen
	// uncached work waits; ordinary scrolling triggers its next admission.
	if _, exists := a.diagrams.entries[key]; !exists && !visible {
		previous := diagramKey{code: key.code, dark: !key.dark}
		if result := a.diagrams.entries[previous]; result != nil && result.err == nil {
			key = previous // Preserve offscreen geometry until the new theme is needed.
		} else if geometry.height == 0 {
			a.codeBlock(c, b)
			return
		}
	}
	result, admitted := a.diagrams.entries[key]
	if !admitted && (visible || geometry.height > 0) {
		admitted = true
	}
	if !admitted {
		a.codeBlock(c, b)
		return
	}

	if result == nil && geometry.height > 0 {
		a.diagramHeader(c, b)
		ui.Column(c).Width(geometry.width).MaxWidthPercent(100).AspectRatio(geometry.width / geometry.height).Role(ui.RoleImage).Label("Mermaid 图表")
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
	geometry.width, geometry.height = result.width, result.height
	if geometry.height == 0 { // Test/custom backends may supply only an image.
		if bitmap, ok := result.image.(*ui.Bitmap); ok {
			w, h := bitmap.Size()
			if w > 0 {
				geometry.height = float32(h) * result.width / float32(w)
			}
		}
	}
	a.diagramHeader(c, b)
	ui.Image(c, result.image).Width(result.width).MaxWidthPercent(100).Label("Mermaid 图表").Role(ui.RoleImage)
}

func (a *readerWindow) beginDiagramFrame(c *ui.Context) func() {
	if a.diagrams == nil {
		return func() {}
	}
	service := a.diagrams
	wanted := make(map[diagramKey]float32)
	a.diagramFrame = wanted
	services := c.Services()
	return func() {
		c.AfterPresent(func() {
			before, bytes := service.next, service.bytes
			service.update(wanted)
			if service.next != before || service.bytes != bytes {
				services.Invalidate()
			}
		})
	}
}

func (a *readerWindow) diagramHeader(c *ui.Context, b *markdown.CodeBlockView) {
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Mermaid").TextColor(c.Theme().TextMuted)
		ui.Spacer(c)
		b.CopyButton(c)
	})
}
