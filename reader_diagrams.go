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
	result, admitted := a.diagrams.request(diagramKey{b.Code(), c.Theme().Dark})
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
