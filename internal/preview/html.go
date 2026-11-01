// Package preview shares MyGo's Markdown parser with the Quick Look renderer.
package preview

import (
	"bytes"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"mdflick/internal/document"
	"mdflick/internal/mermaid"
	"mdflick/internal/typography"
)

// HTML sanitizes Markdown and authorizes only the bundled Mermaid scripts.
// Raw HTML, active URLs, remote images and CSS from documents cannot execute
// or contact the network inside Quick Look.
func HTML(source string) string {
	parsed := parser.Parse(source)
	var codeBlocks []bool
	for i := 0; i < parsed.Len(); i++ {
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
			codeBlocks = append(codeBlocks, document.IsMermaid(parsed.Block(i)))
		}
	}
	codeIndex := 0
	root, _ := html.Parse(strings.NewReader(parsed.HTML()))
	var body bytes.Buffer
	var sanitize func(*html.Node, *html.Node)
	allowed := " p h1 h2 h3 h4 h5 h6 hr pre code blockquote ul ol li strong em del a img table thead tbody tr th td br sup sub input div span "
	sanitize = func(n, parent *html.Node) {
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
				if strings.HasPrefix(a.Val, "#") {
					clean.Attr = append(clean.Attr, a)
				}
			case "src":
				if strings.HasPrefix(a.Val, "data:image/png;base64,") || strings.HasPrefix(a.Val, "data:image/jpeg;base64,") {
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
	}
	container := &html.Node{Type: html.ElementNode, Data: "div"}
	sanitize(root, container)
	for c := container.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&body, c)
	}
	scripts, policy := "", ""
	if document.HasMermaid(parsed) {
		scripts, policy = mermaid.Scripts()
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; ` + policy + `"><style>` + typography.CSS() + `
</style></head><body>` + body.String() + scripts + `</body></html>`
}
