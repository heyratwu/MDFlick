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

type readerHTMLState struct {
	source, markdown, references string
	ranges                       []document.HTMLAlignment
	scrollTo                     string
}

func (s *readerHTMLState) prepare(source string) string {
	if s.source != source {
		s.source = source
		s.markdown, s.ranges = document.NativeHTML(source)
		s.references = ""
		s.scrollTo = ""
		if len(s.ranges) > 0 {
			s.references = readerReferences(parser.Parse(s.markdown))
		}
	}
	return s.markdown
}

// Keep the whole document in the outer Markdown view for heading navigation.
// Aligned prose inherits native text alignment; headings keep stable boxes
// so links inside that prose can scroll to headings elsewhere in the document.
func (a *readerWindow) readerBlock(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	if b.Kind != parser.Paragraph && b.Kind != parser.Heading {
		return a.wideBlock(c, d, i)
	}
	k := sort.Search(len(a.html.ranges), func(k int) bool { return a.html.ranges[k].End > b.Start })
	aligned := k < len(a.html.ranges) && a.html.ranges[k].Start <= b.Start
	if !aligned && (b.Kind != parser.Heading || len(a.html.ranges) == 0) {
		return a.wideBlock(c, d, i)
	}
	align := ui.Start
	alignment := ""
	if aligned {
		alignment = a.html.ranges[k].Align
	}
	switch alignment {
	case "center":
		align = ui.Center
	case "right":
		align = ui.End
	}
	box := ui.Column(c).TextAlign(align)
	if b.Kind == parser.Heading && a.html.scrollTo != "" && markdown.Slug(parser.PlainText(d.Inlines(i))) == a.html.scrollTo {
		box.ScrollIntoView()
		a.html.scrollTo = ""
	}
	state := *ui.Local(box, readerHTMLKey{}, func() *markdown.Options { return &markdown.Options{} })
	state.Theme, state.LoadImage, state.Extensions = a.md.Theme, a.md.LoadImage, d.Extensions()
	services, onLink := c.Services(), a.md.OnLink
	state.OnLink = func(dest string) {
		if onLink != nil {
			onLink(dest)
			return
		}
		if strings.HasPrefix(dest, "#") {
			name, err := url.PathUnescape(dest[1:])
			if err == nil {
				a.html.scrollTo = strings.ToLower(name)
			}
		} else if ui.SafeToOpen(dest) {
			services.OpenURL(dest)
		}
	}
	box.Children(func() { markdown.View(c, d.Source()[b.Start:b.End]+"\n\n"+a.html.references, state) })
	return true
}

type readerHTMLKey struct{}
