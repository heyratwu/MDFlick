package preview

import (
	"strings"
	"testing"
)

func TestPassiveMarkdown(t *testing.T) {
	got := HTML("# 标题\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n```go\nfmt.Println(1)\n```\n\n<script>alert(1)</script>\n\n<img src=\"https://example.com/track\" onerror=\"alert(2)\">\n\n[bad](javascript:alert)\n")
	for _, want := range []string{"<h1>标题</h1>", "<table>", "<pre><code>", "checkbox", "Content-Security-Policy"} {
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
	got := HTML(source)
	for _, want := range []string{`class="mermaid-source"`, "A[中文] --&gt; B[预览]", "script-src 'sha256-", "window.mdflickSVG", "window.mdflickPNG", "println(1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "<script>alert('injected')</script>") || strings.Contains(got, `<script src=`) {
		t.Fatal("document script or remote runtime in output")
	}
	// Ordinary code fences retain the passive preview, with no JS bundled.
	plain := HTML("```go\nprintln(1)\n```\n")
	if strings.Contains(plain, "<script>") || strings.Contains(plain, `class="mermaid-source"`) {
		t.Fatal("ordinary code interpreted as Mermaid")
	}
}
