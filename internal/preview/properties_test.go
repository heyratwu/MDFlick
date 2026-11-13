package preview

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewPropertiesInBothRenderPaths(t *testing.T) {
	for _, tc := range []struct{ filename, title, body string }{
		{"SKILL.md", "SKILL 信息", "# Body"},
		{"README.md", "文档属性", "# Body\n\n<div>HTML body</div>"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			result, err := RenderFile(context.Background(), "---\nname: playwright\ndescription: '<script>alert(1)</script>'\nmetadata:\n  author: test\n---\n\n"+tc.body, tc.filename)
			if err != nil {
				t.Fatal(err)
			}
			card := strings.Index(result, `<section class="document-properties"><div class="properties-title">`+tc.title+`</div>`)
			heading := strings.Index(result, `<h1 id="body">Body</h1>`)
			if card < 0 || heading < card || strings.Contains(result, "<script>") || strings.Count(result, "&lt;script&gt;") != 1 || strings.Contains(result, "查看 YAML") {
				t.Fatal("metadata card was misplaced or emitted unescaped content")
			}
			for _, want := range []string{`<dt>name</dt><dd>playwright</dd>`, `<details><summary>1 个字段</summary>`} {
				if !strings.Contains(result, want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
}
