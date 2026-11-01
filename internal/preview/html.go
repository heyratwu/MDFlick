// Package preview shares MyGo's Markdown parser with the Quick Look renderer.
package preview

import (
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
// Parsing remains synchronous. Passive AST output checks cancellation while
// writing; raw HTML uses the full-document DOM sanitizer.
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
	started = time.Now()
	var codeBlocks []bool
	requiresDOM := false
	diagrams := 0
	for i := 0; i < parsed.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if parsed.Block(i).Kind == parser.HTMLBlock {
			requiresDOM = true
		}
		for _, in := range parsed.Inlines(i) {
			if in.Kind == parser.HTML || (parsed.Block(i).Kind == parser.Heading && in.Kind == parser.FootnoteRef) {
				requiresDOM = true
			}
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
	diagnostics.Record(ctx, "preview.inspect", started, len(source))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !requiresDOM {
		return renderDirect(ctx, parsed, limit, codeBlocks, diagrams)
	}
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
	if err := sanitizeDOM(ctx, root, codeBlocks); err != nil {
		return "", err
	}

	diagnostics.Record(ctx, "preview.sanitize", started, len(generated))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	started = time.Now()
	head, scripts := previewEnvelope(diagrams)
	fixedBytes := len(head) + len(scripts) + len(tail)
	if fixedBytes > limit.outputBytes {
		return "", document.ResourceLimit
	}
	var output strings.Builder
	// Use the generated length as an estimate, bounded by the output allowance.
	// The body is serialized straight into the final allocation, without a copy.
	output.Grow(fixedBytes + min(len(generated), limit.outputBytes-fixedBytes))
	output.WriteString(head)
	writer := &boundedWriter{ctx: ctx, writer: &output, remaining: limit.outputBytes - fixedBytes}
	// A single DocumentNode render shares one serialization buffer across all
	// blocks; per-child Render calls otherwise each allocate a new bufio.Writer.
	if err := html.Render(writer, root); err != nil {
		return "", err
	}
	output.WriteString(scripts)
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

const tail = `</body></html>`

func previewEnvelope(diagrams int) (head, scripts string) {
	policy := ""
	if diagrams > 0 {
		scripts, policy = mermaid.Scripts()
	}
	head = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; ` + policy + `"><style>` + typography.CSS() + `
</style></head><body>`

	return head, scripts
}
