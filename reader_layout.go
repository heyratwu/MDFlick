package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/typography"
)

const readerTextWidth float32 = 820

const readerTitleBarHeight = 40

func (a *readerWindow) titleBar(c *ui.Context) {
	bar := c.TitleBar()
	if bar.Height == 0 { // Native window controls hide in full screen.
		return
	}
	title := "MDFlick"
	if a.path != "" {
		title = filepath.Base(a.path) + " — MDFlick"
	}
	// Equal insets keep the title centered while clearing native controls.
	inset := max(bar.Left, bar.Right) + 12
	ui.Row(c).Height(readerTitleBarHeight).Shrink(0).Padding(0, inset).
		Background(ui.Hex("#F4F3F4")).Center().DragWindow().Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(ui.Hex("#4A4A4A")).MaxWidth(600)
	})
}

// Keep prose centered, but reserve the space to its right for wide blocks.
func readerWidths(c *ui.Context) (left, prose, wide float32) {
	window, _ := c.Size()
	prose = min(readerTextWidth, max(0, window-144))
	left = (window - prose) / 2
	wide = max(prose, window-left-24)
	return
}

func (a *readerWindow) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	_, prose, wide := readerWidths(c)
	prose = max(0, prose-a.blockIndent)
	wide = max(prose, wide-a.blockIndent)
	box := markdown.DefaultCodeBlock(c, b)
	size := *ui.Local(box, readerCodeKey{}, func() *readerCodeSize { return &readerCodeSize{} })
	if source := b.Code(); size.source != source {
		size.source, size.width = source, 0
		for line := range strings.SplitSeq(source, "\n") {
			w, _ := c.MeasureText(0, ui.Span{Text: line, Font: typography.CodeFont, Size: 14, Weight: 400})
			size.width = max(size.width, w+30) // Code padding and border.
		}
	}
	box.Width(min(max(prose, size.width), wide))
}

type readerCodeKey struct{}
type readerCodeSize struct {
	source string
	width  float32
}

// MyGo's table renderer owns its horizontal scroll view. Give that view the
// available right-hand space without widening the surrounding prose.
func (a *readerWindow) wideBlock(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	a.blockIndent = 0
	for parent := b.Parent; parent > 0; parent = d.Block(parent).Parent {
		switch d.Block(parent).Kind {
		case parser.List:
			a.blockIndent += 16 * a.md.Theme.ListIndent
		case parser.Quote:
			a.blockIndent += a.md.Theme.QuoteBarWidth + 16*.85
		}
	}
	if b.Kind != parser.Table {
		return false
	}
	_, _, wide := readerWidths(c)
	box := ui.Column(c).Width(max(0, wide-a.blockIndent))
	state := *ui.Local(box, readerTableKey{}, func() *readerTableState { return &readerTableState{} })
	if state.source != d.Source() {
		state.source = d.Source()
		state.markdown = readerTableMarkdown(d, i)
	}
	state.options.Theme = a.md.Theme
	state.options.OnLink = a.md.OnLink
	state.options.LoadImage = a.md.LoadImage
	state.options.Extensions = d.Extensions()
	box.Children(func() { markdown.View(c, state.markdown, &state.options) })
	return true
}

type readerTableKey struct{}
type readerTableState struct {
	source, markdown string
	options          markdown.Options
}

func readerTableMarkdown(d *parser.Document, i int) string {
	b := d.Block(i)
	refs := readerReferences(d)
	// Rebuild rows from cell source ranges to remove list/quote prefixes.
	var table strings.Builder
	for row := range d.Children(i) {
		table.WriteByte('|')
		for cell := range d.Children(row) {
			cb := d.Block(cell)
			table.WriteString(d.Source()[cb.Start:cb.End])
			table.WriteByte('|')
		}
		table.WriteByte('\n')
		if d.Block(row).Header {
			table.WriteByte('|')
			for _, align := range b.Align {
				switch align {
				case parser.AlignCenter:
					table.WriteString(":---:|")
				case parser.AlignRight:
					table.WriteString("---:|")
				case parser.AlignLeft:
					table.WriteString(":---|")
				default:
					table.WriteString("---|")
				}
			}
			table.WriteByte('\n')
		}
	}
	return table.String() + "\n" + refs
}

func readerReferences(d *parser.Document) string {
	// Keep reference definitions available to blocks rendered in nested views.
	var refs strings.Builder
	end := 0
	for j := range d.Children(0) {
		block := d.Block(j)
		gap := strings.TrimLeft(d.Source()[end:block.Start], " \t|>")
		if parser.Parse(gap).Len() == 1 {
			refs.WriteString(gap)
			refs.WriteByte('\n')
		}
		end = block.End
	}
	if gap := strings.TrimLeft(d.Source()[end:], " \t|>"); parser.Parse(gap).Len() == 1 {
		refs.WriteString(gap)
	}
	return refs.String()
}
