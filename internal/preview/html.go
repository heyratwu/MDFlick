// Package preview shares MyGo's Markdown parser with the Quick Look renderer.
package preview

import (
	"bytes"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"mdflick/internal/mermaid"
)

// HTML sanitizes Markdown and authorizes only the bundled Mermaid scripts.
// Raw HTML, active URLs, remote images and CSS from documents cannot execute
// or contact the network inside Quick Look.
func HTML(source string) string {
	root, _ := html.Parse(strings.NewReader(parser.Parse(source).HTML()))
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
		// Only parser-generated fenced Mermaid blocks receive this marker.
		// Raw document HTML is escaped by the Markdown parser.
		if n.Data == "pre" && n.FirstChild != nil && n.FirstChild.Data == "code" {
			for _, attr := range n.FirstChild.Attr {
				if attr.Key == "class" && attr.Val == "language-mermaid" {
					clean.Attr = append(clean.Attr, html.Attribute{Key: "class", Val: "mermaid-source"})
				}
			}
		}
		for _, a := range n.Attr {
			switch a.Key {
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
	if strings.Contains(body.String(), `class="mermaid-source"`) {
		scripts, policy = mermaid.Scripts()
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; ` + policy + `"><style>
:root{color-scheme:light dark}body{font:16px/1.65 -apple-system,BlinkMacSystemFont,sans-serif;margin:32px auto;padding:0 28px;max-width:900px;overflow-wrap:anywhere}h1,h2{line-height:1.25;border-bottom:1px solid #8885;padding-bottom:.3em}pre,code{font-family:ui-monospace,monospace;background:#8882;border-radius:5px}code{padding:.15em .3em}pre{padding:16px;overflow:auto}pre code{background:none;padding:0}blockquote{margin-left:0;padding-left:18px;border-left:4px solid #8886;color:GrayText}table{border-collapse:collapse;display:block;overflow:auto}th,td{border:1px solid #8885;padding:8px 12px}tr:nth-child(even){background:#8881}img{max-width:100%}a{color:#3984dc}
pre.mermaid-rendered{background:transparent;font-family:inherit;white-space:normal;text-align:center}pre.mermaid-rendered svg{max-width:100%;height:auto}
</style></head><body>` + body.String() + scripts + `</body></html>`
}
