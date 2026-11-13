package document

import (
	"strings"
	"testing"
)

func TestSplitFrontMatter(t *testing.T) {
	header := "\ufeff---\r\nname: \"playwright\"\r\ndescription: >-\r\n  Browser\r\n  automation\r\ntags: [browser, test]\r\nmetadata:\r\n  version: '1.0'\r\n---\r\n"
	body := "# Playwright\r\n"
	properties, rest := SplitFrontMatter(header + body)
	if properties == nil || rest != body || len(properties.Fields) != 4 {
		t.Fatalf("properties = %#v, body = %q", properties, rest)
	}
	if properties.Fields[0].Name != "name" || properties.Fields[0].Text != "playwright" || properties.Fields[1].Text != "Browser automation" {
		t.Fatalf("scalar fields = %#v", properties.Fields[:2])
	}
	if text, inline := properties.Fields[2].DisplayText(); !inline || text != "browser · test" {
		t.Fatalf("list display = %q, %v", text, inline)
	}
	if text, inline := properties.Fields[3].DisplayText(); inline || text != "" || properties.Fields[3].Children[0].Text != "1.0" {
		t.Fatalf("nested field = %#v", properties.Fields[3])
	}
	if properties, rest := SplitFrontMatter("---\nname: only\n---"); properties == nil || rest != "" {
		t.Fatal("metadata-only file was not recognized")
	}
}

func TestSplitFrontMatterLeavesMarkdownIntact(t *testing.T) {
	for name, source := range map[string]string{
		"separator and heading": "---\nA heading\n---\n# Body",
		"invalid YAML":          "---\nname: [unclosed\n---\n# Body",
		"duplicate keys":        "---\nname: one\nname: two\n---\n# Body",
		"unclosed":              "---\nname: test\n# Body",
		"mid-document":          "# Body\n---\nname: test\n---",
		"empty":                 "---\n# Comment\n---\n# Body",
		"sequence":              "---\n- one\n- two\n---\n# Body",
		"oversized":             "---\nname: " + strings.Repeat("a", maxFrontMatterBytes) + "\n---\n# Body",
		"truncated fence":       "---\nname: " + strings.Repeat("a", maxFrontMatterBytes-14) + "\n---invalid\n# Body",
	} {
		t.Run(name, func(t *testing.T) {
			properties, rest := SplitFrontMatter(source)
			if properties != nil || rest != source {
				t.Fatal("ordinary Markdown changed")
			}
		})
	}
}
