package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/typography"
)

// Match GenOffice's default document, while retaining the system appearance.
// MyGo's native block layout cannot express all CSS margin/line-box rules.
func readerTheme(dark bool) *markdown.Theme {
	p := typography.Colors(dark)
	th := &markdown.Theme{
		Font: typography.Font, CodeFont: typography.CodeFont,
		FontSize: 16, LineHeight: 1.75, CodeLineHeight: 1.6,
		Text: ui.Hex(p.Text), Strong: ui.Hex(p.Text), StrongWeight: 700,
		Muted: ui.Hex(p.Muted), Link: ui.Hex(p.Link),
		QuoteText: ui.Hex(p.Secondary), QuoteBar: ui.Hex(p.QuoteBar), QuoteBarWidth: 3,
		Rule: ui.Hex(p.Border), BlockSpacing: .6, ItemSpacing: .15, ListIndent: 1.6,
		Code: ui.Hex(p.Text), CodeSize: .875, CodeBackground: ui.RGBA(129, 139, 152, .14),
		CodeBlockBackground: ui.Hex(p.CodeBackground), CodeBlockBorder: ui.Hex(p.Border),
		CodeBlockRadius: 8, CodeBlockPadding: 1,
		TableBorder: ui.Hex(p.QuoteBar), TableHeader: ui.Hex(p.CodeBackground),
		TableCellPadding: [2]float32{.625, .375},
	}
	if dark {
		th.CodeBackground = ui.RGBA(129, 139, 152, .22)
		th.TableHeader = ui.Hex("#282c32")
	}
	for i, size := range [6]float32{2, 1.5, 1.25, 1.1, .83, .67} {
		th.Headings[i] = markdown.HeadingStyle{
			Size: size, Weight: 650, Color: th.Text, Top: 1.4 * size, Bottom: .5 * size, Rule: i == 0,
		}
	}
	return th
}
