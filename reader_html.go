package main

import (
	"net/url"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/document"
)

type readerHTMLState struct{ scrollTo string }

// Keep the whole document in the outer Markdown view for heading navigation.
// Aligned prose inherits native text alignment; headings keep stable boxes
// so links inside that prose can scroll to headings elsewhere in the document.
func (a *readerWindow) readerBlock(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	if b.Kind != parser.Paragraph && b.Kind != parser.Heading {
		return a.wideBlock(c, d, i)
	}
	k := sort.Search(len(a.prepared.ranges), func(k int) bool { return a.prepared.ranges[k].End > b.Start })
	aligned := k < len(a.prepared.ranges) && a.prepared.ranges[k].Start <= b.Start
	if !aligned && b.Kind != parser.Heading {
		return a.wideBlock(c, d, i)
	}
	align := ui.Start
	alignment := ""
	if aligned {
		alignment = a.prepared.ranges[k].Align
	}
	switch alignment {
	case "center":
		align = ui.Center
	case "right":
		align = ui.End
	}
	box := ui.Column(c).TextAlign(align)
	if b.Kind == parser.Heading && a.html.scrollTo != "" && document.HeadingAnchor(parser.PlainText(d.Inlines(i))) == a.html.scrollTo {
		box.ScrollIntoView()
		a.html.scrollTo = ""
	}
	state := *ui.Local(box, readerHTMLKey{}, func() *markdown.Options { return &markdown.Options{} })
	state.Theme, state.LoadImage, state.Extensions = a.md.Theme, a.md.LoadImage, d.Extensions()
	state.OnLink = a.md.OnLink

	box.Children(func() { markdown.View(c, a.prepared.prose[b.Start], state) })
	return true
}

type readerHTMLKey struct{}

// Every native Markdown surface routes links through the same policy. Anchor
// requests are owned by the window, including links inside nested table views.
func (a *readerWindow) linkHandler(c *ui.Context) func(string) {
	services := c.Services()
	return func(dest string) {
		action := document.NativePolicy().Link(dest)
		if action == document.IgnoreLink {
			return
		}
		if a.onLink != nil {
			a.onLink(dest)
			return
		}
		switch action {
		case document.ScrollToAnchor:
			name, _ := url.PathUnescape(dest[1:])
			a.html.scrollTo = strings.ToLower(name)
		case document.OpenExternal:
			services.OpenURL(dest)
		}
	}
}
