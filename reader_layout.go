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
const readerHighlightLimit = 32 << 10

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
	if source := b.Code(); len(source) > readerHighlightLimit {
		// Avoid a synchronous MeasureText for every line (or one enormous line).
		box.Width(wide)
		return
	} else if size.source != source {
		size.source, size.width = source, 0
		for line := range strings.SplitSeq(source, "\n") {
			if len(line) > 4096 {
				size.width = 1e9
				break
			}
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

// Configure the original block container. Tables retain the full document's
// references, selection and copy handlers instead of creating nested views.
func (a *readerWindow) styleWideBlock(c *ui.Context, d *parser.Document, i int, box ui.Element) {
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
	if b.Kind == parser.Table {
		_, _, wide := readerWidths(c)
		box.Width(max(0, wide-a.blockIndent))
	}
}
