// Package preview shares MyGo's Markdown parser with the Quick Look renderer.
package preview

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/mermaid"
	"mdflick/internal/typography"
)

// Limits bound generated output and interactive diagrams in Quick Look.
const Timeout = document.ProcessingTimeout
const MaxHTMLSize = document.MaxDerivedSize
const MaxDiagrams = 128

// Baselines show that many small blocks amplify DOM allocations. Reject before
// HTML/DOM construction, while retaining the separate 32 MiB input ceiling.
const MaxBlocks = document.MaxBlocks

type limits struct{ outputBytes, diagrams int }

// Render produces a bounded passive preview. Cancellation is cooperative:
// parser.Parse and parser.HTML are synchronous, with checks between stages.
func Render(ctx context.Context, source string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	return render(ctx, source, limits{MaxHTMLSize, MaxDiagrams})
}

func render(ctx context.Context, source string, limit limits) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(source) > document.MaxSize {
		return "", document.TooLarge
	}
	started := time.Now()
	processing := document.QuickLookLimits()
	parsed, err := processing.Parse(ctx, source)
	if err != nil {
		return "", err
	}
	diagnostics.Record(ctx, "preview.parse", started, len(source))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if parsed.Len() > MaxBlocks {
		return "", document.ResourceLimit
	}
	var codeBlocks []bool
	diagrams := 0
	for i := 0; i < parsed.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if parsed.Block(i).Kind != parser.CodeBlock {
			continue
		}
		// HTML moves footnote definitions into a section that this passive
		// adapter omits. Do not let their blocks shift ordinary code markers.
		inFootnote := false
		for parent := parsed.Block(i).Parent; parent >= 0; parent = parsed.Block(parent).Parent {
			if parsed.Block(parent).Kind == parser.FootnoteDefinition {
				inFootnote = true
				break
			}
		}
		if !inFootnote {
			eligible := document.QuickLookPolicy().Mermaid && document.IsMermaid(parsed.Block(i)) && diagrams < limit.diagrams
			codeBlocks = append(codeBlocks, eligible)
			if eligible {
				diagrams++
			}
		}
	}
	codeIndex := 0
	started = time.Now()
	generated := parsed.HTML()
	diagnostics.Record(ctx, "preview.markdown-html", started, len(generated))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(generated) > limit.outputBytes {
		return "", document.ResourceLimit
	}
	started = time.Now()
	root, err := html.Parse(contextReader{ctx, strings.NewReader(generated)})
	diagnostics.Record(ctx, "preview.dom", started, len(generated))
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	started = time.Now()
	var body bytes.Buffer
	var sanitize func(*html.Node, *html.Node)
	anchors := make(map[string]bool)
	allowed := " p h1 h2 h3 h4 h5 h6 hr pre code blockquote ul ol li strong em del a img table thead tbody tr th td br sup sub input div span "
	sanitize = func(n, parent *html.Node) {
		if ctx.Err() != nil {
			return
		}
		if n.Type == html.TextNode {
			parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
			return
		}
		if n.Type != html.ElementNode || n.Data == "html" || n.Data == "body" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				sanitize(c, parent)
			}
			return
		}
		if !strings.Contains(allowed, " "+n.Data+" ") {
			return
		}
		clean := &html.Node{Type: html.ElementNode, Data: n.Data, DataAtom: n.DataAtom}
		// Only fenced Markdown code blocks receive a Mermaid marker.
		if n.Data == "pre" && n.FirstChild != nil && n.FirstChild.Data == "code" {
			if codeIndex < len(codeBlocks) && codeBlocks[codeIndex] {
				clean.Attr = append(clean.Attr, html.Attribute{Key: "class", Val: "mermaid-source"})
			}
			codeIndex++
		}
		for _, a := range n.Attr {
			switch a.Key {
			case "align":
				align := strings.ToLower(strings.TrimSpace(a.Val))
				if align == "left" || align == "center" || align == "right" {
					clean.Attr = append(clean.Attr, html.Attribute{Key: "align", Val: align})
				}
			case "alt", "title", "colspan", "rowspan", "start", "checked":
				clean.Attr = append(clean.Attr, a)
			case "href":
				if document.QuickLookPolicy().Link(a.Val) == document.ScrollToAnchor {
					clean.Attr = append(clean.Attr, a)
				}
			case "src":
				if document.QuickLookPolicy().AllowsImage(a.Val) {
					clean.Attr = append(clean.Attr, a)
				}
			}
		}
		if n.Data == "input" {
			clean.Attr = append(clean.Attr, html.Attribute{Key: "type", Val: "checkbox"}, html.Attribute{Key: "disabled", Val: ""})
		}
		parent.AppendChild(clean)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			sanitize(c, clean)
		}
		if len(clean.Data) == 2 && clean.Data[0] == 'h' && clean.Data[1] >= '1' && clean.Data[1] <= '6' {
			anchor := document.HeadingAnchor(headingText(clean))
			// Native anchors select the first matching heading.
			if anchor != "" && !anchors[anchor] {
				clean.Attr = append(clean.Attr, html.Attribute{Key: "id", Val: anchor})
				anchors[anchor] = true
			}
		}
	}
	container := &html.Node{Type: html.ElementNode, Data: "div"}
	sanitize(root, container)
	diagnostics.Record(ctx, "preview.sanitize", started, len(generated))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	started = time.Now()
	writer := &boundedWriter{ctx: ctx, writer: &body, remaining: limit.outputBytes}
	for c := container.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(writer, c); err != nil {
			return "", err
		}
	}
	scripts, policy := "", ""
	if diagrams > 0 {
		scripts, policy = mermaid.Scripts()
	}
	head := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; ` + policy + `"><style>` + typography.CSS() + `
</style></head><body>`
	tail := scripts + `</body></html>`
	if len(head)+body.Len()+len(tail) > limit.outputBytes {
		return "", document.ResourceLimit
	}
	var output strings.Builder
	output.Grow(len(head) + body.Len() + len(tail))
	output.WriteString(head)
	output.Write(body.Bytes())
	output.WriteString(tail)
	diagnostics.Record(ctx, "preview.output", started, output.Len())
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return output.String(), nil
}

// Check cancellation during the HTML parser's reads and serialized output.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > 64<<10 {
		p = p[:64<<10]
	}
	return r.reader.Read(p)
}

type boundedWriter struct {
	ctx       context.Context
	writer    io.Writer
	remaining int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > w.remaining {
		return 0, document.ResourceLimit
	}
	n, err := w.writer.Write(p)
	w.remaining -= n
	return n, err
}

func headingText(n *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			text.WriteByte(' ')
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "alt" {
					text.WriteString(a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return text.String()
}
