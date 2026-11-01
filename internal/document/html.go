package document

import (
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
)

// HTMLAlignment records the text alignment inherited from an HTML container.
// Offsets refer to the converted Markdown, so code fences remain untouched.
type HTMLAlignment struct {
	Start, End int
	Align      string
}

type htmlContainer struct{ tag, align string }

// NativeHTML translates passive HTML blocks into Markdown for the native view.
// The parser, rather than a source-wide replacement, keeps literal code intact.
func NativeHTML(source string) (string, []HTMLAlignment) {
	if !strings.Contains(source, "<") {
		return source, nil
	}
	d := parser.Parse(source)
	var out strings.Builder
	var ranges []HTMLAlignment
	var stack []htmlContainer
	alignment := func() string {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].align != "" {
				return stack[i].align
			}
		}
		return ""
	}
	write := func(s string) {
		if s == "" {
			return
		}
		start := out.Len()
		out.WriteString(s)
		if align := alignment(); align != "" {
			if len(ranges) > 0 && ranges[len(ranges)-1].End == start && ranges[len(ranges)-1].Align == align {
				ranges[len(ranges)-1].End = out.Len()
			} else {
				ranges = append(ranges, HTMLAlignment{start, out.Len(), align})
			}
		}
	}
	end := 0
	for i := 1; i < d.Len(); i++ {
		b := d.Block(i)
		if b.Kind != parser.HTMLBlock {
			continue
		}
		write(source[end:b.Start])
		z := html.NewTokenizer(strings.NewReader(d.Lines(i)))
		pre, skip := false, ""
		var links []string
		for {
			kind := z.Next()
			if kind == html.ErrorToken {
				break
			}
			raw := string(z.Raw())
			token := z.Token()
			if skip != "" {
				if kind == html.EndTagToken && token.Data == skip {
					skip = ""
				}
				continue
			}
			switch kind {
			case html.TextToken:
				// Preserve entity spelling for Markdown's inline parser.
				if pre {
					write(token.Data)
				} else {
					write(raw)
				}
			case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
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
					if !closing {
						skip = tag
					}
				case "div", "p", "center", "section", "article", "header", "footer":
					write("\n\n")
					if closing {
						for j := len(stack) - 1; j >= 0; j-- {
							if stack[j].tag == tag {
								stack = stack[:j]
								break
							}
						}
					} else if kind != html.SelfClosingTagToken {
						align := strings.ToLower(strings.TrimSpace(attr("align")))
						if tag == "center" {
							align = "center"
						}
						if align != "left" && align != "center" && align != "right" {
							align = ""
						}
						stack = append(stack, htmlContainer{tag, align})
					}
				case "h1", "h2", "h3", "h4", "h5", "h6":
					write("\n\n")
					if !closing {
						write(strings.Repeat("#", int(tag[1]-'0')) + " ")
					}
				case "br":
					write("  \n")
				case "hr":
					write("\n\n---\n\n")
				case "strong", "b":
					write("**")
				case "em", "i":
					write("*")
				case "del", "s":
					write("~~")
				case "pre":
					if closing {
						write("\n```\n\n")
						pre = false
					} else {
						write("\n\n```\n")
						pre = true
					}
				case "code":
					if !pre {
						write("`")
					}
				case "a":
					if closing {
						if len(links) > 0 {
							write("](" + links[len(links)-1] + ")")
							links = links[:len(links)-1]
						}
					} else {
						write("[")
						links = append(links, attr("href"))
					}
				case "img":
					write("![" + attr("alt") + "](" + attr("src") + ")")
				case "ul", "ol":
					write("\n\n")
				case "li":
					if closing {
						write("\n")
					} else {
						write("\n- ")
					}
				default:
					// Native Markdown already handles passive inline formatting tags.
					if tag == "sub" || tag == "sup" || tag == "mark" {
						write(raw)
					}
				}
			}
		}
		end = b.End
	}
	write(source[end:])
	return out.String(), ranges
}
