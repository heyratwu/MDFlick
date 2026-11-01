package testfixtures

import "strings"

// ResourceCases is the contract matrix used by both actual display adapters.
// Quick Look always retains alt text and never loads document images.
type ResourceCase struct {
	Name, Destination                   string
	NativeImage, NativeExternal, Anchor bool
}

var ResourceCases = []ResourceCase{
	{"https", "https://example.com/image.png", true, true, false},
	{"http", "http://example.com/image.png", true, true, false},
	{"local", "image.png", true, false, false},
	{"file", "file:/tmp/image.png", true, false, false},
	{"relative-document", "other.md", true, false, false},
	{"mail", "mailto:hello@example.com", false, true, false},
	{"anchor", "#destination", true, false, true},
	{"script", "javascript:alert", false, false, false},
	{"data", "data:image/png;base64,AAAA", false, false, false},
	{"protocol-relative", "//example.com/image.png", false, false, false},
}

type PerformanceCase struct{ Name, Source string }

func PerformanceCases() []PerformanceCase {
	const paragraph = "A paragraph of plain Markdown text with **emphasis** and a [link](https://example.com).\n\n"
	return []PerformanceCase{
		{"text-1MiB", strings.Repeat(paragraph, (1<<20)/len(paragraph))},
		{"tables-500", strings.Repeat("| Name | Value |\n|---|---:|\n| [Reference][r] | 123 |\n\n", 500) + "[r]: https://example.com\n"},
		{"images-1000", strings.Repeat("![local](image.png) ![remote](https://example.com/image.png)\n\n", 1000)},
		{"diagrams-256", strings.Repeat("```mermaid\nflowchart LR\n A --> B\n```\n\n", 256)},
		{"html-500", strings.Repeat("<div align=center>\n\n## Heading\n\n**Paragraph** with [link][r].\n\n</div>\n\n", 500) + "[r]: https://example.com\n"},
	}
}
