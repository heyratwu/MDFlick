package preview

import (
	"context"
	"strings"

	"golang.org/x/net/html"
	"mdflick/internal/document"
)

// sanitizeDOM reuses the parsed nodes and attribute storage. Unknown elements
// lose their entire subtree; only document/html/body wrappers are unwrapped.
// It is private to a render request: a cancelled tree is never published.
func sanitizeDOM(ctx context.Context, root *html.Node, codeBlocks []bool) error {
	codeIndex := 0
	anchors := make(map[string]bool)
	policy := document.QuickLookPolicy()
	var children func(*html.Node) error
	children = func(parent *html.Node) error {
		for n := parent.FirstChild; n != nil; {
			if err := ctx.Err(); err != nil {
				return err
			}
			next := n.NextSibling // Removal/unwrapping changes sibling links.
			switch {
			case n.Type == html.TextNode:
			case n.Type != html.ElementNode || n.Data == "html" || n.Data == "body":
				if err := children(n); err != nil {
					return err
				}
				for n.FirstChild != nil {
					child := n.FirstChild
					n.RemoveChild(child)
					parent.InsertBefore(child, n)
				}
				parent.RemoveChild(n)
			case !passiveElement(n.Data):
				parent.RemoveChild(n)
			default:
				// The previous clone kept Data/DataAtom but not the element namespace.
				n.Namespace = ""
				marked := false
				if n.Data == "pre" && n.FirstChild != nil && n.FirstChild.Data == "code" {
					marked = codeIndex < len(codeBlocks) && codeBlocks[codeIndex]
					codeIndex++
				}
				original := n.Attr
				attrs := original[:0]
				for _, a := range original {
					switch a.Key {
					case "align":
						align := strings.ToLower(strings.TrimSpace(a.Val))
						if align == "left" || align == "center" || align == "right" {
							attrs = append(attrs, html.Attribute{Key: "align", Val: align})
						}
					case "alt", "title", "colspan", "rowspan", "start", "checked":
						attrs = append(attrs, a)
					case "href":
						if policy.Link(a.Val) == document.ScrollToAnchor {
							attrs = append(attrs, a)
						}
					case "src":
						if policy.AllowsImage(a.Val) {
							attrs = append(attrs, a)
						}
					}
				}
				// Drop references to rejected attribute values in the reused backing array.
				clear(original[len(attrs):])
				if marked {
					attrs = append(attrs, html.Attribute{})
					copy(attrs[1:], attrs[:len(attrs)-1])
					attrs[0] = html.Attribute{Key: "class", Val: "mermaid-source"}
				}
				if n.Data == "input" {
					attrs = append(attrs, html.Attribute{Key: "type", Val: "checkbox"}, html.Attribute{Key: "disabled", Val: ""})
				}
				n.Attr = attrs
				if err := children(n); err != nil {
					return err
				}
				if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
					anchor := document.HeadingAnchor(headingText(n))
					if anchor != "" && !anchors[anchor] {
						n.Attr = append(n.Attr, html.Attribute{Key: "id", Val: anchor})
						anchors[anchor] = true
					}
				}
			}
			n = next
		}
		return ctx.Err()
	}
	return children(root)
}

func passiveElement(name string) bool {
	switch name {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "pre", "code",
		"blockquote", "ul", "ol", "li", "strong", "em", "del", "a", "img",
		"table", "thead", "tbody", "tr", "th", "td", "br", "sup", "sub",
		"input", "div", "span":
		return true
	}
	return false
}
