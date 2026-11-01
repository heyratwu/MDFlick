package preview

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/testfixtures"
)

// Compare browser DOMs with the old parser->HTML->sanitize path, rather than
// asserting the new writer's implementation. Formatting whitespace between
// structural blocks is immaterial; text, attributes and nesting must agree.
func TestDirectMatchesPassiveDOM(t *testing.T) {
	sources := []string{
		"# 中文 **Title** & ![alt *text*](x)\n\n# 中文 **Title** & ![alt *text*](x)\n\n[Go](#中文-title)\n\n---\n",
		"Text **bold** *em* ~~strike~~ `a < b` & \\\"quote\\\".\nSoft line  \nHard break\n\n![image *alt*](https://example.com/x \"title & quote\")\n",
		"3. Three\n4. Four\n   - Nested\n   - [x] Checked\n\n> Quote\n>\n> Paragraph\n\n- loose\n\n  paragraph\n\n  ```go\n  <x> & 'quote'\n  ```\n",
		"| Left | Center | Right |\n|:---|:---:|---:|\n| [A](#a) | `x` | ![alt](x) |\n",
		"A[^n] and B[^other] and A[^n].\n\n[^n]: note\n\n    ```mermaid\n    flowchart LR; X-->Y\n    ```\n\n[^other]: Other.\n\n```mermaid\nflowchart LR; A-->B\n```\n",
		"Math \\(a < b\\).\n\n$$\na < b\n$$\n",
	}
	for _, tc := range testfixtures.PerformanceCases() {
		if tc.Name != "html-500" {
			sources = append(sources, tc.Source)
		}
	}
	for _, source := range sources {
		d := parser.Parse(source)
		var markers []bool
		for i := 0; i < d.Len(); i++ {
			b := d.Block(i)
			if b.Kind != parser.CodeBlock {
				continue
			}
			footnote := false
			for p := b.Parent; p >= 0; p = d.Block(p).Parent {
				if d.Block(p).Kind == parser.FootnoteDefinition {
					footnote = true
					break
				}
			}
			if !footnote {
				count := 0
				for _, marked := range markers {
					if marked {
						count++
					}
				}
				markers = append(markers, document.IsMermaid(b) && count < MaxDiagrams)
			}
		}
		root, err := html.Parse(strings.NewReader(d.HTML()))
		if err != nil {
			t.Fatal(err)
		}
		if err = sanitizeDOM(context.Background(), root, markers); err != nil {
			t.Fatal(err)
		}
		var legacy strings.Builder
		if err = html.Render(&legacy, root); err != nil {
			t.Fatal(err)
		}
		got := renderHTML(t, source)
		body := got[strings.Index(got, "<body>")+6 : strings.LastIndex(got, "</body>")]
		if end := strings.Index(body, "<script>"); end >= 0 {
			body = body[:end]
		}
		if a, b := canonicalBody(t, body), canonicalBody(t, legacy.String()); a != b {
			t.Fatalf("passive DOM mismatch for %.100s\ngot %.1500s\nwant %.1500s", source, a, b)
		}
	}
}

func canonicalBody(t *testing.T, s string) string {
	t.Helper()
	root, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	var normalize func(*html.Node)
	normalize = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.TextNode && n.Data != "code" && n.Data != "pre" {
				if strings.TrimSpace(c.Data) == "" && (n.Data == "body" || n.Data == "ul" || n.Data == "ol" || n.Data == "li" || n.Data == "blockquote" || n.Data == "table" || n.Data == "thead" || n.Data == "tbody" || n.Data == "tr") {
					n.RemoveChild(c)
				}
			}
			normalize(c)
			c = next
		}
	}
	normalize(root)
	var out strings.Builder
	if err = html.Render(&out, root); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestPreviewChoosesDirectOrDOM(t *testing.T) {
	for _, tc := range []struct {
		source string
		dom    bool
	}{
		{"# Normal\n\nLiteral < text and ` <b> `.", false},
		{"<div align=center>\n\n# Heading\n\n</div>", true},
		{"# Inline <span>HTML</span>", true},
		{"# Heading[^n]\n\n[^n]: Note", true},
		{"<script>drop</script>\n\nKeep", true},
	} {
		stages := map[string]bool{}
		ctx := diagnostics.WithObserver(context.Background(), func(e diagnostics.Event) { stages[e.Stage] = true })
		if _, err := Render(ctx, tc.source); err != nil {
			t.Fatal(err)
		}
		if stages["preview.dom"] != tc.dom || stages["preview.direct"] == tc.dom {
			t.Fatal("wrong output path", tc.source, stages)
		}
	}
}

func TestDirectCancellationDuringWrites(t *testing.T) {
	ctx := &sanitizeCancelContext{Context: context.Background(), remaining: 40}
	d := parser.Parse(strings.Repeat("Paragraph **bold**.\n\n", 1000))
	result, err := renderDirect(ctx, d, limits{MaxHTMLSize, MaxDiagrams}, nil, 0)
	if err != context.Canceled || result != "" {
		t.Fatal("partial direct result published", err)
	}
}
