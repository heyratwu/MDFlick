package main

import (
	"net/url"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/document"
)

type readerHTMLState struct{ scrollTo string }

// Style blocks in the one document view, preserving references and selection.
func (a *readerWindow) readerBlockStyle(c *ui.Context, d *parser.Document, i int, box ui.Element) {
	a.styleWideBlock(c, d, i, box)
	b := d.Block(i)
	if b.Kind != parser.Paragraph && b.Kind != parser.Heading {
		return
	}
	k := sort.Search(len(a.prepared.ranges), func(k int) bool { return a.prepared.ranges[k].End > b.Start })
	if k < len(a.prepared.ranges) && a.prepared.ranges[k].Start <= b.Start {
		switch a.prepared.ranges[k].Align {
		case "center":
			box.TextAlign(ui.Center)
		case "right":
			box.TextAlign(ui.End)
		}
	}
	if b.Kind == parser.Heading && a.html.scrollTo != "" && document.HeadingAnchor(parser.PlainText(d.Inlines(i))) == a.html.scrollTo {
		box.ScrollIntoView()
		a.html.scrollTo = ""
	}
}

// Every native Markdown surface routes links through the same policy. Anchor
// requests are owned by the window, including links inside table cells.
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
