package preview

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
)

// Plain Markdown has no browser HTML repair semantics to preserve. Emit only
// passive markup straight from the AST, without a generated HTML string or DOM.
// Documents containing raw HTML retain the full-document sanitizer: filtering
// isolated tags would break containers spanning multiple Markdown blocks.
func renderDirect(ctx context.Context, d *parser.Document, limit limits, code []bool, diagrams int) (string, error) {
	started := time.Now()
	head, scripts := previewEnvelope(diagrams)
	fixed := len(head) + len(scripts) + len(tail)
	if fixed > limit.outputBytes {
		return "", document.ResourceLimit
	}
	var output strings.Builder
	output.Grow(fixed + min(len(d.Source()), limit.outputBytes-fixed))
	output.WriteString(head)
	w := passiveWriter{d: d, ctx: ctx, builder: &output, remaining: limit.outputBytes - fixed, code: code, anchors: make(map[string]bool)}
	w.blocks(0)
	if w.err != nil {
		return "", w.err
	}
	output.WriteString(scripts)
	output.WriteString(tail)
	diagnostics.Record(ctx, "preview.direct", started, output.Len())
	if err := ctx.Err(); err != nil {
		return "", err
	}
	diagnostics.Record(ctx, "preview.output", started, output.Len())
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return output.String(), nil
}

type passiveWriter struct {
	d         *parser.Document
	ctx       context.Context
	builder   *strings.Builder
	remaining int
	last      byte
	err       error
	code      []bool
	codeIndex int
	anchors   map[string]bool
	notes     []string
}

func (w *passiveWriter) write(s string) {
	if w.err != nil {
		return
	}
	// strings.Builder implements WriteString; avoid a temporary []byte per tag.
	if w.err = w.ctx.Err(); w.err != nil {
		return
	}
	if len(s) > w.remaining {
		w.err = document.ResourceLimit
		return
	}
	_, w.err = w.builder.WriteString(s)
	w.remaining -= len(s)
	if len(s) > 0 {
		w.last = s[len(s)-1]
	}
}

func (w *passiveWriter) cr() {
	if w.last != 0 && w.last != '\n' {
		w.write("\n")
	}
}

func (w *passiveWriter) escape(s string) {
	start := 0
	for i := 0; i < len(s) && w.err == nil; i++ {
		var entity string
		switch s[i] {
		case '&':
			entity = "&amp;"
		case '<':
			entity = "&lt;"
		case '>':
			entity = "&gt;"
		case '"':
			entity = "&#34;"
		case '\'':
			entity = "&#39;"
		case '\r':
			entity = "&#13;"
		case 0:
			entity = "�"
		}
		if entity != "" {
			w.write(s[start:i])
			w.write(entity)
			start = i + 1
		}
		if i-start >= 32<<10 {
			w.write(s[start : i+1])
			start = i + 1
		}
	}
	w.write(s[start:])
}

func (w *passiveWriter) blocks(parent int) {
	for i := range w.d.Children(parent) {
		if w.err != nil {
			return
		}
		w.block(i)
	}
}

func (w *passiveWriter) block(i int) {
	b := w.d.Block(i)
	switch b.Kind {
	case parser.Paragraph:
		tight := b.Parent > 0 && w.d.Block(b.Parent).Kind == parser.Item && w.d.Block(w.d.Block(b.Parent).Parent).Tight
		if !tight {
			w.cr()
			w.write("<p>")
		}
		w.inlines(i)
		if !tight {
			w.write("</p>\n")
		}
	case parser.Heading:
		w.cr()
		tag := "h" + strconv.Itoa(b.Level)
		w.write("<" + tag)
		anchor := document.HeadingAnchor(passiveHeadingText(w.d.Inlines(i)))
		if anchor != "" && !w.anchors[anchor] {
			w.anchors[anchor] = true
			w.write(` id="`)
			w.escape(anchor)
			w.write(`"`)
		}
		w.write(">")
		w.inlines(i)
		w.write("</" + tag + ">\n")
	case parser.ThematicBreak:
		w.cr()
		w.write("<hr/>\n")
	case parser.CodeBlock:
		w.cr()
		marked := w.codeIndex < len(w.code) && w.code[w.codeIndex]
		w.codeIndex++
		if marked {
			w.write(`<pre class="mermaid-source"><code>`)
		} else {
			w.write("<pre><code>")
		}
		for line := 0; line < w.d.LineCount(i) && w.err == nil; line++ {
			s, _ := w.d.Line(i, line)
			w.escape(s)
			w.write("\n")
		}
		w.write("</code></pre>\n")
	case parser.MathBlock:
		w.cr()
		w.write("<div>")
		w.escape(strings.TrimSuffix(w.d.Lines(i), "\n"))
		w.write("</div>\n")
	case parser.Quote:
		w.cr()
		w.write("<blockquote>\n")
		w.blocks(i)
		w.write("</blockquote>\n")
	case parser.List:
		w.cr()
		tag := "ul"
		if b.Ordered {
			tag = "ol"
		}
		w.write("<" + tag)
		if b.Ordered && b.Number != 1 {
			w.write(` start="` + strconv.Itoa(b.Number) + `"`)
		}
		w.write(">\n")
		w.blocks(i)
		w.write("</" + tag + ">\n")
	case parser.Item:
		w.cr()
		w.write("<li>")
		if b.Task == parser.Checked {
			w.write(`<input checked="" type="checkbox" disabled=""/> `)
		}
		if b.Task == parser.Unchecked {
			w.write(`<input type="checkbox" disabled=""/> `)
		}
		w.blocks(i)
		w.write("</li>\n")
	case parser.Table:
		w.cr()
		w.write("<table>\n")
		body := false
		for r := range w.d.Children(i) {
			if w.err != nil {
				return
			}
			row := w.d.Block(r)
			if row.Header {
				w.write("<thead>\n")
			} else if !body {
				w.write("<tbody>\n")
				body = true
			}
			w.write("<tr>\n")
			k := 0
			for cell := range w.d.Children(r) {
				if w.err != nil {
					return
				}
				tag := "td"
				if row.Header {
					tag = "th"
				}
				w.write("<" + tag)
				if k < len(b.Align) {
					switch b.Align[k] {
					case parser.AlignLeft:
						w.write(` align="left"`)
					case parser.AlignCenter:
						w.write(` align="center"`)
					case parser.AlignRight:
						w.write(` align="right"`)
					}
				}
				w.write(">")
				w.inlines(cell)
				w.write("</" + tag + ">\n")
				k++
			}
			w.write("</tr>\n")
			if row.Header {
				w.write("</thead>\n")
			}
		}
		if body {
			w.write("</tbody>\n")
		}
		w.write("</table>\n")
	// Footnote bodies were omitted by the passive DOM policy (section subtree).
	case parser.FootnoteDefinition:
	}
}

func (w *passiveWriter) inlines(i int) { in := w.d.Inlines(i); w.inline(in, 0, len(in)) }
func (w *passiveWriter) inline(in []parser.Inline, from, to int) {
	for k := from; k < to && w.err == nil; k = in[k].Next {
		n := in[k]
		switch n.Kind {
		case parser.Text:
			w.escape(n.Text)
		case parser.SoftBreak:
			w.write("\n")
		case parser.LineBreak:
			w.write("<br/>\n")
		case parser.Code:
			w.write("<code>")
			w.escape(n.Text)
			w.write("</code>")
		case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Link:
			tag := "em"
			switch n.Kind {
			case parser.Strong:
				tag = "strong"
			case parser.Strikethrough:
				tag = "del"
			case parser.Link:
				tag = "a"
			}
			w.write("<" + tag)
			if n.Kind == parser.Link {
				if document.QuickLookPolicy().Link(n.Dest) == document.ScrollToAnchor {
					w.write(` href="`)
					w.href(n.Dest)
					w.write(`"`)
				}
				if n.Title != "" {
					w.write(` title="`)
					w.escape(n.Title)
					w.write(`"`)
				}
			}
			w.write(">")
			w.inline(in, k+1, n.Next)
			w.write("</" + tag + ">")
		case parser.Image:
			w.write(`<img alt="`)
			w.escape(parser.PlainText(in[k+1 : n.Next]))
			w.write(`"`)
			if n.Title != "" {
				w.write(` title="`)
				w.escape(n.Title)
				w.write(`"`)
			}
			w.write("/>")
		case parser.Math:
			w.write("<span>")
			w.escape(n.Text)
			w.write("</span>")
		case parser.FootnoteRef:
			label := parser.NormalizeLabel(n.Text)
			number := 0
			for i, note := range w.notes {
				if note == label {
					number = i + 1
					break
				}
			}
			if number == 0 {
				w.notes = append(w.notes, label)
				number = len(w.notes)
			}
			id := strconv.Itoa(number)
			w.write(`<sup><a href="#fn-` + id + `">` + id + `</a></sup>`)
		}
	}
}

// Match the parser's URL encoding before HTML attribute escaping.
func (w *passiveWriter) href(s string) {
	const hex = "0123456789ABCDEF"
	start := 0
	for i := 0; i < len(s) && w.err == nil; i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.+!*(),%#@?=;:/,+$~", rune(c)) {
			continue
		}
		w.escape(s[start:i])
		if c == '&' || c == '\'' {
			w.escape(s[i : i+1])
		} else {
			w.write(string([]byte{'%', hex[c>>4], hex[c&15]}))
		}
		start = i + 1
	}
	w.escape(s[start:])
}

// DOM headings count soft newlines as whitespace removed by HeadingAnchor,
// hard breaks as spaces, and footnote references by their displayed number.
func passiveHeadingText(in []parser.Inline) string {
	var b strings.Builder
	for _, n := range in {
		switch n.Kind {
		case parser.Text, parser.Code, parser.Math:
			b.WriteString(n.Text)
		case parser.SoftBreak:
			b.WriteByte('\n')
		case parser.LineBreak:
			b.WriteString(" \n")
		case parser.FootnoteRef:
			return "" // Handled by the DOM path; see inspection.
		}
	}
	return b.String()
}
