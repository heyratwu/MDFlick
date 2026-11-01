package main

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReaderWideBlocks(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/wide.md", source: "# Reading\n\n" + strings.Repeat("Readable prose with a comfortable line length. ", 15) + "\n\n```go\n" + strings.Repeat("long_code_", 30) + "\n```\n\n|" + strings.Repeat(" Column |", 12) + "\n|" + strings.Repeat(" --- |", 12) + "\n|" + strings.Repeat(" content |", 11) + " finalcell |" + "\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1600, 1000)
	for _, width := range []int{1600, 2560, 900, 600, 1600} {
		tt.SetSize(width, 1000)
		r, ok := tt.Find("Reading")
		if !ok || r.W > 820.5 {
			t.Fatalf("window %d: heading bounds %+v", width, r)
		}
		if tt.HasText("\n|\n") {
			t.Fatal("table produced stray source text")
		}
		code, ok := tt.Find("Code, go")
		if !ok || code.X != r.X || code.W <= r.W || code.X+code.W > float32(width)-23 {
			t.Fatalf("window %d: prose %+v, code %+v", width, r, code)
		}
	}
}

func TestReaderNestedCode(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/nested.md", source: "# Reading\n\n> ```go\n> " + strings.Repeat("long_code_", 30) + "\n> ```\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1600, 1000)
	r, _ := tt.Find("Reading")
	code, ok := tt.Find("Code, go")
	if !ok || code.X <= r.X || code.X+code.W > 1576.5 {
		t.Fatalf("nested code escapes the window: %+v", code)
	}
}

func TestWideTableKeepsReferences(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/table.md", source: "| Name |\n|---|\n| [Table link][ref] |\n\n[ref]: https://example.com\n"}}
	clicked := ""
	a.resetMarkdown(context.Background(), a.path)
	a.md.OnLink = func(url string) { clicked = url }
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1600, 1000)
	if err := tt.Click("Table link"); err != nil {
		t.Fatal(err)
	}
	if clicked != "https://example.com" {
		t.Fatalf("table reference resolved to %q", clicked)
	}
	for _, text := range tt.Texts() {
		if text == "|" {
			t.Fatal("table delimiter leaked into the document")
		}
	}
}

func TestWideTableScrollsAtWindowEdge(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/table.md", source: "# Reading\n\n|" + strings.Repeat(" Column |", 20) + "\n|" + strings.Repeat(" --- |", 20) + "\n|" + strings.Repeat(" content |", 19) + " finalcell |\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 1000)
	before, ok := tt.Find("finalcell")
	if !ok || before.X < 876 {
		t.Fatalf("expected the last column to overflow: %+v", before)
	}
	header, _ := tt.Find("Column")
	prose, _ := tt.Find("Reading")
	tt.Scroll(header.X+10, header.Y+10, 3000, 0)
	after, _ := tt.Find("finalcell")
	currentProse, _ := tt.Find("Reading")
	if after.X >= before.X || after.X+after.W > 877 || currentProse.X != prose.X {
		t.Fatalf("table should scroll independently: before %+v, after %+v, heading %+v", before, after, currentProse)
	}
}
