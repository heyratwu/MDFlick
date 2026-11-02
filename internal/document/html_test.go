package document

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestNativeHTMLContainers(t *testing.T) {
	source := "<div align='center'>\n\n# Center\n\n<div align=right>\n\nRight\n\n</div>\n\nCenter again\n\n</div>\n\nOutside\n\n```html\n<div align=right>literal</div>\n```\n\n`<div>`\n"
	got, ranges := testNativeHTML(t, source)
	for _, tc := range []struct{ text, align string }{{"# Center", "center"}, {"Right", "right"}, {"Center again", "center"}, {"Outside", ""}, {"```html", ""}} {
		pos := strings.Index(got, tc.text)
		align := ""
		for _, r := range ranges {
			if pos >= r.Start && pos < r.End {
				align = r.Align
			}
		}
		if pos < 0 || align != tc.align {
			t.Errorf("%q alignment = %q, want %q; %s", tc.text, align, tc.align, got)
		}
	}
	if !strings.Contains(got, "```html\n<div align=right>literal</div>\n```") || !strings.Contains(got, "`<div>`") {
		t.Fatal("literal HTML changed", got)
	}
	if strings.Contains(got, "</div>\n\nOutside") {
		t.Fatal("container leaked into prose", got)
	}
}

func TestNativeHTMLFormatting(t *testing.T) {
	got, _ := testNativeHTML(t, "<div align=center><h2>Title</h2><p><strong>Bold</strong> &amp; <a href=\"https://example.com\">link</a><br>line</p><script>alert(1)</script></div>\n")
	rendered := parser.Parse(got).HTML()
	for _, want := range []string{"<h2>Title</h2>", "<strong>Bold</strong>", `href="https://example.com"`, "&amp;", "<br"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in %s", want, rendered)
		}
	}
	if strings.Contains(got, "alert") || strings.Contains(got, "<div") {
		t.Fatal(got)
	}
}

// A deterministic cancellation context makes a large conversion stop while
// processing tokens, rather than relying on timing or a second goroutine.
type conversionContext struct {
	context.Context
	checks, cancelAt int
}

func (c *conversionContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestNativeHTMLContextLimitsAndCancellation(t *testing.T) {
	d := parser.Parse("<div align=center>" + strings.Repeat("<b>text</b>", 100) + "</div>")
	ctx := &conversionContext{Context: context.Background(), cancelAt: 25}
	output, ranges, err := NativeHTMLContext(ctx, d, NativeLimits())
	if err != context.Canceled || output != "" || len(ranges) != 0 {
		t.Fatalf("conversion did not cancel atomically: %q %+v %v", output, ranges, err)
	}
	limits := NativeLimits()
	// Horizontal rules expand into Markdown, while the original input fits.
	d = parser.Parse(strings.Repeat("<hr>", 10))
	limits.ConvertedBytes = len(d.Source())
	output, ranges, err = NativeHTMLContext(context.Background(), d, limits)
	if err != ResourceLimit || output != "" || len(ranges) != 0 {
		t.Fatalf("conversion output limit: %q %v", output, err)
	}
}

func testNativeHTML(t *testing.T, source string) (string, []HTMLAlignment) {
	t.Helper()
	result, ranges, err := NativeHTMLContext(context.Background(), parser.Parse(source), NativeLimits())
	if err != nil {
		t.Fatal(err)
	}
	return result, ranges
}

func TestNativeHTMLReadmeImages(t *testing.T) {
	source := `<h1 align="center">
  <br>
  <a href="https://example.com/project">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://example.com/dark.webp">
      <source media="(prefers-color-scheme: light)" srcset="https://example.com/light.webp">
      <img src="https://example.com/fallback.webp" alt="Logo" width="400" title="Project">
    </picture>
  </a>
  <br>
  <a href="https://example.com/program"><img src="https://example.com/badge.svg" alt="Badge"></a>
</h1>`
	result, err := NativeHTMLDocumentContext(context.Background(), parser.Parse(source), NativeLimits())
	if err != nil {
		t.Fatal(err)
	}
	d := parser.Parse(result.Markdown)
	count := 0
	for i := 1; i < d.Len(); i++ {
		if d.Block(i).Kind == parser.CodeBlock {
			t.Fatalf("HTML indentation became code: %q", result.Markdown)
		}
		for _, in := range d.Inlines(i) {
			if in.Kind == parser.Image {
				count++
				im, ok := result.Images[in.Start]
				if !ok {
					t.Fatalf("missing metadata at %d: %+v", in.Start, result.Images)
				}
				if in.Dest == "https://example.com/fallback.webp" && (im.Width != 400 || im.Title != "Project" || im.SourceForTheme(true) != "https://example.com/dark.webp" || im.SourceForTheme(false) != "https://example.com/light.webp") {
					t.Fatal(im)
				}
			}
		}
	}
	if count != 2 {
		t.Fatalf("images=%d: %s", count, result.Markdown)
	}
	if len(result.Ranges) == 0 || result.Ranges[0].Align != "center" {
		t.Fatal(result.Ranges)
	}
}

func TestNativeHTMLSubset(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		wants        []string
	}{
		{"ordered-nested", `<ol start="3"><li>First<ul><li>Nested</li></ul></li><li>Second</li></ol>`, []string{`<ol start="3">`, `<ul>`, `<li>Nested</li>`, `Second`}},
		{"table", `<table><thead><tr><th>Name</th><th>Link</th></tr></thead><tbody><tr><td>A | B</td><td><a href="https://example.com">Visit</a></td></tr></tbody></table>`, []string{`<table>`, `<th>Name</th>`, `A | B`, `href="https://example.com"`}},
		{"quote", `<blockquote><p>Quoted <strong>words</strong>.</p><p>Again</p></blockquote>`, []string{`<blockquote>`, `<strong>words</strong>`, `Again`}},
		{"formatting", `<div><u>underline</u> <kbd>Cmd</kbd> <sup>2</sup> <mark>highlight</mark></div>`, []string{`<u>underline</u>`, `<kbd>Cmd</kbd>`, `<sup>2</sup>`, `<mark>highlight</mark>`}},
		{"pre", "<pre><code>&lt;img&gt;\n  indented *literal*\n</code></pre>", []string{`&lt;img&gt;`, `  indented *literal*`}},
		{"literal-text", `<p>A *literal* [label] and a_b</p>`, []string{`A *literal* [label] and a_b`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := testNativeHTML(t, tc.source)
			rendered := parser.Parse(got).HTML()
			for _, want := range tc.wants {
				if !strings.Contains(rendered, want) {
					t.Errorf("missing %q in %s; markdown=%q", want, rendered, got)
				}
			}
		})
	}
}

func TestNativeHTMLInlineImagesAndCode(t *testing.T) {
	source := "[![Markdown](m.png)](https://example.com)\n\n<a href=\"https://example.com\"><img src=\"a(b).png\" width=\"50%\" alt=\"A [B]\"></a>\n\n`<img src=\"literal.png\">`\n\n```html\n<img src=\"fenced.png\">\n```"
	result, err := NativeHTMLDocumentContext(context.Background(), parser.Parse(source), NativeLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 {
		t.Fatal(result.Images)
	}
	d := parser.Parse(result.Markdown)
	found := false
	for i := 1; i < d.Len(); i++ {
		for _, in := range d.Inlines(i) {
			if in.Kind == parser.Image && in.Dest == "a(b).png" {
				found = true
				im := result.Images[in.Start]
				if im.WidthPercent != 50 {
					t.Fatal(im)
				}
			}
		}
	}
	if !found || !strings.Contains(result.Markdown, "`<img src=\"literal.png\">`") || !strings.Contains(result.Markdown, "<img src=\"fenced.png\">") {
		t.Fatal(result.Markdown)
	}
}

func TestNativeHTMLActiveSubtreesAndAttributes(t *testing.T) {
	source := `Before <script>hidden()</script> after.

<template>

# Hidden heading

</template>

<div>Visible <a href="javascript:alert(1)">unsafe</a><img src="javascript:alert(1)" width="NaN"><p>Tail</p></div>`
	result, err := NativeHTMLDocumentContext(context.Background(), parser.Parse(source), NativeLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"hidden()", "Hidden heading"} {
		if strings.Contains(result.Markdown, hidden) {
			t.Fatalf("active content leaked: %s", result.Markdown)
		}
	}
	if !strings.Contains(result.Markdown, "Visible") || !strings.Contains(result.Markdown, "Tail") {
		t.Fatal(result.Markdown)
	}
	for _, im := range result.Images {
		if im.Width != 0 || NativePolicy().AllowsImage(im.Source) {
			t.Fatal(im)
		}
	}
}

func TestNativeHTMLListParagraphsAndLiteralFences(t *testing.T) {
	source := "<ol start=\"10\"><li><p>First paragraph</p><p>Second paragraph</p><ul><li>Nested item</li></ul>Tail</li><li>Last item</li></ol>\n\n<pre><code>````\nexample\n````\n</code></pre>"
	converted, _ := testNativeHTML(t, source)
	rendered := parser.Parse(converted).HTML()
	for _, want := range []string{`<ol start="10">`, "<p>First paragraph</p>", "<p>Second paragraph</p>", "<ul>", "Nested item", "Tail", "Last item", "<code>````\nexample\n````"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q: %s; markdown=%q", want, rendered, converted)
		}
	}
	if strings.Count(rendered, "<ol") != 1 || strings.Count(rendered, "<ul>") != 1 {
		t.Fatal(rendered)
	}
}
