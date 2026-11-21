package main

import (
	_ "embed"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/typography"
)

const readerTextWidth float32 = 820
const readerHighlightLimit = 32 << 10

const readerTitleBarHeight = 40

//go:embed resources/text-wrap.svg
var readerWrapSVG []byte

var (
	readerCopyIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.6V3.2A1.7 1.7 0 0 0 8.8 1.5H4.2A1.7 1.7 0 0 0 2.5 3.2v4.6a1.7 1.7 0 0 0 1.7 1.7h.4"/></svg>`))
	readerCheckIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg>`))
	readerWrapIcon  = ui.MustParseSVG(readerWrapSVG)
)

func (a *readerWindow) title() string {
	if a.path != "" {
		return filepath.Base(a.path) + " — MDFlick"
	}
	if a.sidebar.root != "" {
		return filepath.Base(a.sidebar.root) + " — MDFlick"
	}
	return "MDFlick"
}

func (a *readerWindow) titleBar(c *ui.Context) {
	bar := c.TitleBar()
	if bar.Height == 0 {
		return
	}
	background, foreground := ui.Hex("#F4F3F4"), ui.Hex("#4A4A4A")
	if c.Theme().Dark {
		background, foreground = ui.Hex("#2B2B2B"), ui.Hex("#E4E4E4")
	}
	// Equal insets keep the title centered while clearing native controls.
	rightInset := bar.Right + 52
	if a.textPreview != nil {
		rightInset += 32
	}
	inset := max(bar.Left+52, rightInset)
	ui.Row(c).Height(readerTitleBarHeight).Shrink(0).Padding(0, inset).
		Background(background).Center().DragWindow().Children(func() {
		if a.path != "" || a.sidebar.root != "" {
			button := ui.ButtonBase(c).Key("toggle-sidebar").Absolute().Left(bar.Left+12).Top(6).
				Size(28, 28).Radius(5).Center().Label("切换侧边栏").Tooltip("切换侧边栏").OnClick(a.toggleSidebar)
			if button.Hovered() {
				button.Background(foreground.Alpha(0.08))
			}
			button.Children(func() { ui.Icon(c, readerSidebarIcon).Size(16, 16).TextColor(foreground) })
		}
		ui.Text(c, a.title()).FontSize(13).FontWeight(500).TextColor(foreground).MaxWidth(600)
		if a.path != "" && !isPreviewImagePath(a.path) && !a.loading && a.failure == "" {
			copyLabel := "复制 Markdown 全文"
			if a.textPreview != nil {
				copyLabel = "复制文件全文"
				wrap := ui.ToggleBase(c.Key("toggle-text-wrap"), &a.textWrap).Absolute().Right(bar.Right+44).Top(6).
					Size(28, 28).Radius(5).Label("自动换行").Tooltip("自动换行")
				if a.textWrap || wrap.Hovered() {
					wrap.Background(foreground.Alpha(0.08))
				}
				wrap.Children(func() { ui.Icon(c, readerWrapIcon).Size(16, 16).TextColor(foreground) })
			}
			icon := readerCopyIcon
			if !a.copiedAt.IsZero() && c.Now().Sub(a.copiedAt) < 2*time.Second {
				icon = readerCheckIcon
			}
			button := ui.ButtonBase(c).Key("copy-document").Absolute().Right(bar.Right+12).Top(6).
				Size(28, 28).Radius(5).Center().Label(copyLabel).Tooltip(copyLabel)
			if button.Hovered() {
				button.Background(foreground.Alpha(0.08))
			}
			button.Children(func() {
				ui.Icon(c, icon).Size(16, 16).TextColor(foreground)
			})
			if button.Clicked() {
				c.WriteClipboard(a.source)
				a.copiedAt = c.Now()
				c.After(2 * time.Second)
			}
		}
	})
}

// Keep prose centered, but reserve the space to its right for wide blocks.
func readerWidths(c *ui.Context) (left, prose, wide float32) {
	window, _ := c.Size()
	return readerWidthsFor(window)
}

func (a *readerWindow) readerWidths(c *ui.Context) (left, prose, wide float32) {
	window, _ := c.Size()
	return readerWidthsFor(window - a.sidebarWidth(c))
}

func readerWidthsFor(window float32) (left, prose, wide float32) {
	prose = min(readerTextWidth, max(0, window-144))
	left = (window - prose) / 2
	wide = max(prose, window-left-24)
	return
}

func (a *readerWindow) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	_, prose, wide := a.readerWidths(c)
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
		_, _, wide := a.readerWidths(c)
		box.Width(max(0, wide-a.blockIndent))
	}
}
