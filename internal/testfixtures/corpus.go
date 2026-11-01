// Package testfixtures supplies the same Markdown corpus to both adapters.
package testfixtures

import _ "embed"

//go:embed markdown/reader.md
var reader string

//go:embed markdown/diagrams.md
var diagrams string

//go:embed markdown/passive.md
var passive string

type Case struct {
	Name, Source  string
	MermaidBlocks int
}

var Cases = []Case{
	{"footnote-order", "[^note]: note\n\n    ```go\n    println(1)\n    ```\n\nReference[^note].\n\n```mermaid\nflowchart LR\n A --> B\n```\n", 1},
	{"reader", reader, 0},
	{"diagrams", diagrams, 2},
	{"passive", passive, 0},
	{"word-only", "The word mermaid and `mermaid` are ordinary text.\n", 0},
	{"other-language", "```go\n// mermaid\n```\n", 0},
	{"indented", "    ```mermaid\n    flowchart LR\n    A --> B\n    ```\n", 0},
	{"unfinished", "```mermaid\nflowchart LR\nA --> B\n", 0},
	{"case-sensitive", "```Mermaid\nflowchart LR\nA --> B\n```\n", 0},
}
