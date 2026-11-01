package preview

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSanitizeDOMContracts(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		code              []bool
	}{
		{"subtree-removal-and-siblings",
			`<div onclick=x align=" RIGHT "><script>bad</script><!--gone--><p title="keep" style=x>First</p><iframe><p>drop</p></iframe><p>Last</p></div>`,
			`<div align="right"><p title="keep">First</p><p>Last</p></div>`, nil},
		{"wrappers-and-head",
			`<!doctype html><html><head><style>bad</style><title>drop</title></head><body><p>First</p><p>Last</p></body></html>`,
			`<p>First</p><p>Last</p>`, nil},
		{"attributes-inputs-and-heading-text",
			`<input type=password onfocus=x checked name=n value=s><a href="#ok" onclick=x id=doc>Link</a><img src=data:x alt=Alt title=Keep onerror=x><h2 id=doc>Hi<br>There<img src=file:x alt=Pic></h2><h2>Hi<br>There<img alt=Pic></h2>`,
			`<input checked="" type="checkbox" disabled=""/><a href="#ok">Link</a><img alt="Alt" title="Keep"/><h2 id="hi-therepic">Hi<br/>There<img alt="Pic"/></h2><h2>Hi<br/>There<img alt="Pic"/></h2>`, nil},
		{"generated-mermaid-class-only",
			`<pre class=mermaid-source><code class=language-mermaid>x</code></pre><pre align=right onclick=x><code>real</code></pre>`,
			`<pre><code>x</code></pre><pre class="mermaid-source" align="right"><code>real</code></pre>`, []bool{false, true}},
		{"foreign-content-and-unknown-subtree",
			`<svg><a href="#bad">drop</a><foreignObject><p>also drop</p></foreignObject></svg><article><h1>drop</h1></article><p>Keep</p>`,
			`<p>Keep</p>`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := html.Parse(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if err := sanitizeDOM(context.Background(), root, tc.code); err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			if err := html.Render(&output, root); err != nil {
				t.Fatal(err)
			}
			if output.String() != tc.want {
				t.Fatalf("got %s\nwant %s", output.String(), tc.want)
			}
		})
	}
}

type sanitizeCancelContext struct {
	context.Context
	remaining int
}

func (c *sanitizeCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func TestSanitizeCancellationDuringTraversal(t *testing.T) {
	root, err := html.Parse(strings.NewReader(strings.Repeat(`<div><p title=keep>Content</p><script>drop</script></div>`, 100)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &sanitizeCancelContext{Context: context.Background(), remaining: 25}
	if err := sanitizeDOM(ctx, root, nil); err != context.Canceled {
		t.Fatal("traversal did not stop", err)
	}
}
