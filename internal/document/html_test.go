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
