package preview

import (
	"context"
	"strings"
	"testing"

	"mdflick/internal/testfixtures"
)

func TestPassiveMarkdown(t *testing.T) {
	got := renderHTML(t, "# 标题\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n```go\nfmt.Println(1)\n```\n\n<script>alert(1)</script>\n\n<img src=\"https://example.com/track\" onerror=\"alert(2)\">\n\n[bad](javascript:alert)\n")
	for _, want := range []string{"<h1 id=\"标题\">标题</h1>", "<table>", "<pre><code>", "checkbox", "Content-Security-Policy"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s: %s", want, got)
		}
	}
	for _, bad := range []string{"<script", "onerror", "https://example.com", "javascript:"} {
		if strings.Contains(got, bad) {
			t.Errorf("unsafe output: %s", bad)
		}
	}
}

func TestMermaidOfflineAndEscaping(t *testing.T) {
	source := "```mermaid\nflowchart LR\n A[中文] --> B[预览]\n```\n\n```go\nprintln(1)\n```\n\n<script>alert('injected')</script>\n"
	got := renderHTML(t, source)
	for _, want := range []string{`class="mermaid-source"`, "A[中文] --&gt; B[预览]", "script-src 'sha256-", "window.mdflickSVG", "window.mdflickPNG", "println(1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "<script>alert('injected')</script>") || strings.Contains(got, `<script src=`) {
		t.Fatal("document script or remote runtime in output")
	}
	// Ordinary code fences retain the passive preview, with no JS bundled.
	plain := renderHTML(t, "```go\nprintln(1)\n```\n")
	if strings.Contains(plain, "<script>") || strings.Contains(plain, `class="mermaid-source"`) {
		t.Fatal("ordinary code interpreted as Mermaid")
	}
}

func TestSharedCorpus(t *testing.T) {
	for _, tc := range testfixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := renderHTML(t, tc.Source)
			if count := strings.Count(got, `class="mermaid-source"`); count != tc.MermaidBlocks {
				t.Fatalf("marked %d code blocks, expected %d", count, tc.MermaidBlocks)
			}
			if scripts := strings.Contains(got, "<script>"); scripts != (tc.MermaidBlocks > 0) {
				t.Fatal("runtime does not match native detection")
			}
			if tc.Name == "reader" {
				for _, want := range []string{"共同标题", "<table>", "checkbox", "println", `href="#%E5%85%B1%E5%90%8C%E6%A0%87%E9%A2%98"`} {
					if !strings.Contains(got, want) {
						t.Errorf("missing %q", want)
					}
				}
			}
			if tc.Name == "passive" {
				for _, bad := range []string{"<script>", "onerror", "javascript:", "https://example.com"} {
					if strings.Contains(got, bad) {
						t.Errorf("active content survived: %s", bad)
					}
				}
			}
		})
	}
}

func TestHTMLContainerAlignment(t *testing.T) {
	got := renderHTML(t, "<div align=\"center\" onclick=\"alert(1)\">\n\n# 乔木剪藏 · Qiaomu Clipper\n\n**中文** | [English](#english)\n\n</div>\n\nOutside\n\n```html\n<div align=\"center\">literal</div>\n```\n")
	for _, want := range []string{`<div align="center">`, `<h1 id="乔木剪藏--qiaomu-clipper">乔木剪藏 · Qiaomu Clipper</h1>`, `<strong>中文</strong>`, `href="#english"`, `&lt;div align=&#34;center&#34;&gt;literal&lt;/div&gt;`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "onclick") || strings.Contains(got, "alert(1)") {
		t.Fatal("active HTML survived sanitization")
	}
}

func renderHTML(t *testing.T, source string) string {
	t.Helper()
	result, err := Render(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
