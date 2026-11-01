package document

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestNativeHTMLContainers(t *testing.T) {
	source := "<div align='center'>\n\n# Center\n\n<div align=right>\n\nRight\n\n</div>\n\nCenter again\n\n</div>\n\nOutside\n\n```html\n<div align=right>literal</div>\n```\n\n`<div>`\n"
	got, ranges := NativeHTML(source)
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
	got, _ := NativeHTML("<div align=center><h2>Title</h2><p><strong>Bold</strong> &amp; <a href=\"https://example.com\">link</a><br>line</p><script>alert(1)</script></div>\n")
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
