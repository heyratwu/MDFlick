package document

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
)

// Offsets in native HTML metadata refer to the converted Markdown. Keeping
// image attributes separate avoids encoding presentation data into URLs.
type HTMLAlignment struct {
	Start, End int
	Align      string
}
type HTMLImage struct {
	Start                           int
	Source, DarkSource, LightSource string
	Width, Height, WidthPercent     float32
	Title                           string
}

func (im HTMLImage) SourceForTheme(dark bool) string {
	if dark && im.DarkSource != "" {
		return im.DarkSource
	}
	if !dark && im.LightSource != "" {
		return im.LightSource
	}
	return im.Source
}

type NativeHTML struct {
	Markdown string
	Ranges   []HTMLAlignment
	Images   map[int]HTMLImage
}
type htmlContainer struct{ tag, align string }
type htmlList struct {
	ordered           bool
	next              int
	indent            int
	inItem, itemStart bool
}
type htmlPicture struct{ dark, light string }
type nativeHTMLWriter struct {
	ctx                                context.Context
	limit                              ProcessingLimits
	out                                strings.Builder
	ranges                             []HTMLAlignment
	images                             map[int]HTMLImage
	stack                              []htmlContainer
	lists                              []htmlList
	pictures                           []htmlPicture
	links                              []string
	pre, heading                       bool
	skip                               string
	preFence                           string
	skipDepth                          int
	table, tableRow, tableCells, quote int
	err                                error
}

// NativeHTMLContext is the text-only API for callers that don't render images.
func NativeHTMLContext(ctx context.Context, d *parser.Document, limit ProcessingLimits) (string, []HTMLAlignment, error) {
	result, err := NativeHTMLDocumentContext(ctx, d, limit)
	return result.Markdown, result.Ranges, err
}

// NativeHTMLDocumentContext converts HTML blocks and inline tags while leaving
// Markdown text and code untouched. State spans HTML blocks, since a container
// may enclose native Markdown separated by blank lines.
func NativeHTMLDocumentContext(ctx context.Context, d *parser.Document, limit ProcessingLimits) (NativeHTML, error) {
	source := d.Source()
	if err := ctx.Err(); err != nil {
		return NativeHTML{}, err
	}
	if len(source) > limit.ConvertedBytes || d.Len() > limit.Blocks {
		return NativeHTML{}, ResourceLimit
	}
	if !strings.Contains(source, "<") {
		return NativeHTML{Markdown: source}, nil
	}
	w := &nativeHTMLWriter{ctx: ctx, limit: limit, images: make(map[int]HTMLImage)}
	end := 0
	for i := 1; i < d.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return NativeHTML{}, err
		}
		b := d.Block(i)
		if b.Kind == parser.HTMLBlock {
			w.source(source[end:b.Start])
			w.tokens(d.Lines(i))
			end = b.End
		} else {
			// Only recognized raw HTML tokens are transformed. Inline code, fenced
			// code, autolinks, and Markdown punctuation never enter the HTML tokenizer.
			for _, in := range d.Inlines(i) {
				if in.Kind != parser.HTML || in.Start < end {
					continue
				}
				w.source(source[end:in.Start])
				w.tokens(in.Text)
				end = in.End
			}
		}
		if w.err != nil {
			return NativeHTML{}, w.err
		}
	}
	w.source(source[end:])
	if w.err != nil {
		return NativeHTML{}, w.err
	}
	if err := ctx.Err(); err != nil {
		return NativeHTML{}, err
	}
	return NativeHTML{w.out.String(), w.ranges, w.images}, nil
}

// Source gaps contain Markdown, not HTML text. Active subtrees are omitted
// even when the Markdown parser split them across blocks or inline tokens.
func (w *nativeHTMLWriter) source(s string) {
	if w.skip == "" {
		w.write(s)
	}
}

func (w *nativeHTMLWriter) alignment() string {
	for i := len(w.stack) - 1; i >= 0; i-- {
		if w.stack[i].align != "" {
			return w.stack[i].align
		}
	}
	return ""
}
func (w *nativeHTMLWriter) write(s string) {
	if w.err != nil || s == "" {
		return
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return
	}
	if len(s) > w.limit.ConvertedBytes-w.out.Len() {
		w.err = ResourceLimit
		return
	}
	start := w.out.Len()
	w.out.WriteString(s)
	if align := w.alignment(); align != "" {
		if len(w.ranges) > 0 && w.ranges[len(w.ranges)-1].End == start && w.ranges[len(w.ranges)-1].Align == align {
			w.ranges[len(w.ranges)-1].End = w.out.Len()
		} else {
			w.ranges = append(w.ranges, HTMLAlignment{start, w.out.Len(), align})
		}
	}
}
func (w *nativeHTMLWriter) boundary() {
	if w.table > 0 {
		return
	}
	if len(w.links) > 0 || w.heading {
		w.write(" ")
		return
	}
	if len(w.lists) > 0 && w.lists[len(w.lists)-1].inItem {
		if w.lists[len(w.lists)-1].itemStart {
			return
		}
		w.write("\n\n" + strings.Repeat("> ", w.quote) + w.listIndent(len(w.lists)))
		return
	}
	if w.quote > 0 {
		prefix := strings.Repeat("> ", w.quote)
		w.write("\n" + prefix + "\n" + prefix)
	} else {
		w.write("\n\n")
	}
}
func (w *nativeHTMLWriter) listIndent(depth int) string {
	n := 0
	for _, list := range w.lists[:depth] {
		n += list.indent
	}
	return strings.Repeat(" ", n)
}
func (w *nativeHTMLWriter) text(raw string) {
	if w.pre {
		w.write(html.UnescapeString(raw))
		return
	}
	// HTML whitespace is collapsible, unlike Markdown indentation. Keeping it
	// verbatim can turn a deeply indented <img> into an indented code block.
	collapsed := strings.Join(strings.Fields(raw), " ")
	if len(raw) > 0 && unicode.IsSpace(rune(raw[0])) {
		collapsed = " " + collapsed
	}
	if len(raw) > 0 && unicode.IsSpace(rune(raw[len(raw)-1])) && !strings.HasSuffix(collapsed, " ") {
		collapsed += " "
	}
	if w.out.Len() == 0 || strings.HasSuffix(w.out.String(), "\n") || strings.HasSuffix(w.out.String(), " ") {
		collapsed = strings.TrimLeft(collapsed, " ")
	}
	if len(w.lists) > 0 && strings.TrimSpace(collapsed) != "" {
		list := &w.lists[len(w.lists)-1]
		if list.inItem {
			if strings.HasSuffix(w.out.String(), "\n") {
				w.write(strings.Repeat("> ", w.quote) + w.listIndent(len(w.lists)))
			}
			list.itemStart = false
		}
	}
	w.write(escapeHTMLText(collapsed))
}

var htmlTextEscaper = strings.NewReplacer(
	"\\", "&#92;", "*", "&#42;", "_", "&#95;", "[", "&#91;", "]", "&#93;",
	"`", "&#96;", "|", "&#124;", "#", "&#35;", ">", "&gt;", "<", "&lt;",
	"-", "&#45;", "+", "&#43;", ".", "&#46;", "!", "&#33;", "$", "&#36;",
)

func escapeHTMLText(s string) string { return htmlTextEscaper.Replace(s) }
func htmlDestination(s string) string {
	return "<" + strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D").Replace(s) + ">"
}
func (w *nativeHTMLWriter) container(tag string, closing bool, align string) {
	if closing {
		for i := len(w.stack) - 1; i >= 0; i-- {
			if w.stack[i].tag == tag {
				w.stack = w.stack[:i]
				break
			}
		}
	} else {
		align = strings.ToLower(strings.TrimSpace(align))
		if tag == "center" {
			align = "center"
		}
		if align != "left" && align != "center" && align != "right" {
			align = ""
		}
		if len(w.stack) >= w.limit.Blocks {
			w.err = ResourceLimit
			return
		}
		w.stack = append(w.stack, htmlContainer{tag, align})
	}
}
func (w *nativeHTMLWriter) tokens(source string) {
	// A fence longer than every backtick run in this HTML fragment keeps
	// preformatted Markdown examples literal, including embedded fences.
	longest, run := 0, 0
	for _, ch := range source {
		if ch == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	z := html.NewTokenizer(strings.NewReader(source))
	for {
		if err := w.ctx.Err(); err != nil {
			w.err = err
			return
		}
		if w.err != nil {
			return
		}
		kind := z.Next()
		if kind == html.ErrorToken {
			return
		}
		raw := string(z.Raw())
		token := z.Token()
		if w.skip != "" {
			if token.Data == w.skip {
				if kind == html.StartTagToken {
					w.skipDepth++
				}
				if kind == html.EndTagToken {
					w.skipDepth--
					if w.skipDepth == 0 {
						w.skip = ""
					}
				}
			}
			continue
		}
		if kind == html.TextToken {
			w.text(raw)
			continue
		}
		if kind != html.StartTagToken && kind != html.EndTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		tag := token.Data
		closing := kind == html.EndTagToken
		attr := func(key string) string {
			for _, a := range token.Attr {
				if a.Key == key {
					return a.Val
				}
			}
			return ""
		}
		switch tag {
		case "script", "style", "iframe", "object", "template":
			if !closing && kind != html.SelfClosingTagToken {
				w.skip = tag
				w.skipDepth = 1
			}
		case "div", "p", "center", "section", "article", "header", "footer", "figure", "figcaption", "details", "summary":
			w.boundary()
			w.container(tag, closing, attr("align"))
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if closing {
				w.heading = false
				w.boundary()
				w.container(tag, true, "")
			} else {
				w.boundary()
				w.container(tag, false, attr("align"))
				w.write(strings.Repeat("#", int(tag[1]-'0')) + " ")
				w.heading = true
			}
		case "br":
			if w.heading || len(w.links) > 0 || w.table > 0 {
				w.write("<br>")
			} else {
				w.write("  \n")
				if w.quote > 0 {
					w.write(strings.Repeat("> ", w.quote))
				}
			}
		case "hr":
			w.write("\n\n---\n\n")
		case "pre":
			if closing {
				w.write("\n" + w.preFence + "\n\n")
				w.pre = false
			} else {
				w.boundary()
				w.preFence = fence
				w.write(w.preFence + "\n")
				w.pre = true
			}
		case "code":
			if !w.pre {
				w.write(raw)
			}
		case "b", "strong", "i", "em", "u", "ins", "s", "del", "strike", "kbd", "samp", "mark", "sub", "sup", "small":
			w.write(raw)
		case "a":
			if closing {
				if len(w.links) > 0 {
					w.write("](" + htmlDestination(w.links[len(w.links)-1]) + ")")
					w.links = w.links[:len(w.links)-1]
				}
			} else {
				w.write("[")
				w.links = append(w.links, attr("href"))
			}
		case "picture":
			if closing {
				if len(w.pictures) > 0 {
					w.pictures = w.pictures[:len(w.pictures)-1]
				}
			} else {
				w.pictures = append(w.pictures, htmlPicture{})
			}
		case "source":
			if len(w.pictures) > 0 && !closing {
				media := strings.ReplaceAll(strings.ToLower(attr("media")), " ", "")
				src := firstHTMLSource(attr("srcset"))
				pic := &w.pictures[len(w.pictures)-1]
				if media == "(prefers-color-scheme:dark)" && pic.dark == "" {
					pic.dark = src
				}
				if media == "(prefers-color-scheme:light)" && pic.light == "" {
					pic.light = src
				}
			}
		case "img":
			if closing {
				continue
			}
			if len(w.lists) > 0 {
				w.lists[len(w.lists)-1].itemStart = false
			}
			im := HTMLImage{Start: w.out.Len(), Source: attr("src"), Title: attr("title")}
			if im.Source == "" {
				im.Source = firstHTMLSource(attr("srcset"))
			}
			im.Width, im.WidthPercent = htmlDimension(attr("width"), true)
			im.Height, _ = htmlDimension(attr("height"), false)
			if len(w.pictures) > 0 {
				pic := w.pictures[len(w.pictures)-1]
				im.DarkSource, im.LightSource = pic.dark, pic.light
			}
			if len(w.images) >= w.limit.Blocks {
				w.err = ResourceLimit
				return
			}
			w.images[im.Start] = im
			w.write("![" + escapeHTMLText(html.EscapeString(attr("alt"))) + "](" + htmlDestination(im.Source) + ")")
		case "ul", "ol":
			if closing {
				if len(w.lists) > 0 {
					w.lists = w.lists[:len(w.lists)-1]
				}
				w.write("\n")
				if len(w.lists) == 0 {
					w.boundary()
				}
			} else {
				if len(w.lists) == 0 {
					w.boundary()
				} else {
					w.write("\n")
				}
				start, err := strconv.Atoi(attr("start"))
				if err != nil || start < 0 || start > 999999999 {
					start = 1
				}
				w.lists = append(w.lists, htmlList{ordered: tag == "ol", next: start})
			}
		case "li":
			if closing {
				if len(w.lists) > 0 {
					w.lists[len(w.lists)-1].inItem = false
				}
				w.write("\n")
			} else {
				if !strings.HasSuffix(w.out.String(), "\n") {
					w.write("\n")
				}
				w.write(strings.Repeat("> ", w.quote) + w.listIndent(max(0, len(w.lists)-1)))
				marker := "- "
				if len(w.lists) > 0 {
					list := &w.lists[len(w.lists)-1]
					if list.ordered {
						marker = fmt.Sprintf("%d. ", list.next)
						list.next++
					}
				}
				w.write(marker)
				if len(w.lists) > 0 {
					list := &w.lists[len(w.lists)-1]
					list.indent = len(marker)
					list.inItem = true
					list.itemStart = true
				}
			}
		case "blockquote":
			if closing {
				w.quote = max(0, w.quote-1)
				w.boundary()
			} else {
				w.quote++
				w.boundary()
			}
		case "table":
			if closing {
				w.table = 0
				w.boundary()
			} else {
				w.boundary()
				w.table = 1
				w.tableRow = 0
			}
		case "tr":
			if w.table > 0 {
				if closing {
					w.write("\n")
					if w.tableRow == 0 {
						w.write("|" + strings.Repeat(" --- |", w.tableCells) + "\n")
					}
					w.tableRow++
				} else {
					w.tableCells = 0
					w.write("|")
				}
			}
		case "th", "td":
			if w.table > 0 {
				if closing {
					w.write(" |")
				} else {
					w.tableCells++
					w.write(" ")
				}
			}
			// Passive wrappers are unwrapped; their readable descendants are retained.
		}
	}
}
func firstHTMLSource(srcset string) string {
	candidate, _, _ := strings.Cut(srcset, ",")
	fields := strings.Fields(candidate)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}
func htmlDimension(value string, percent bool) (float32, float32) {
	value = strings.TrimSpace(value)
	isPercent := strings.HasSuffix(value, "%")
	if isPercent {
		value = strings.TrimSuffix(value, "%")
	}
	n, err := strconv.ParseFloat(value, 32)
	if err != nil || !(n > 0) || n > 100000 {
		return 0, 0
	}
	if isPercent {
		if percent {
			return 0, float32(min(n, 100))
		}
		return 0, 0
	}
	return float32(n), 0
}
