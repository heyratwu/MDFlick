package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/diagnostics"
)

func TestPreparedViewAdoptsDocumentAndReplacesCaches(t *testing.T) {
	prepared, err := prepareReaderDocument(context.Background(), "# Original\n\nA [reference][r].\n\n[r]: https://example.com\n")
	if err != nil {
		t.Fatal(err)
	}
	current := prepared.parsed
	revisions := make([]uint32, current.Len())
	for i := 1; i < current.Len(); i++ {
		revisions[i] = current.Block(i).Revision()
	}
	var seen *parser.Document
	options := &markdown.Options{BlockStyle: func(_ *ui.Context, d *parser.Document, _ int, _ ui.Element) { seen = d }}
	tt := ui.NewTester(func(c *ui.Context) { markdown.ViewDocument(c, current, options) }, 900, 720)
	if seen != current {
		t.Fatal("view reparsed instead of adopting prepared document")
	}
	for i := 1; i < current.Len(); i++ {
		if current.Block(i).Revision() != revisions[i] {
			t.Fatal("first frame reparsed warmed inlines")
		}
		b := current.Block(i)
		if b.Kind == parser.Heading || b.Kind == parser.Paragraph {
			before := b.Revision()
			tt.Frame()
			if current.Block(i).Revision() != before {
				t.Fatal("inline caches invalidated during rendering")
			}
		}
	}
	current = parser.Parse("# Replaced\n\nA different paragraph.")
	tt.Frame()
	if seen != current || !tt.HasText("Replaced") || tt.HasText("Original") {
		t.Fatal("replaced parse retained stale view caches")
	}
}

func TestReaderUnifiedSelectionCopiesAcrossStyledBlocks(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/copy.md", source: "# Title\n\n<div align=center>\n\nCentered **text**.\n\n</div>\n\n| Key | Value |\n|---|---|\n| one | two |\n\nLast paragraph.\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	r, _ := tt.Find("Centered text.")
	tt.ClickAt(r.X+5, r.Y+r.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	copied := tt.Clipboard()
	for _, want := range []string{"Title", "Centered text.", "Key\tValue", "one\ttwo", "Last paragraph."} {
		if !strings.Contains(copied, want) {
			t.Fatalf("missing %q in unified selection %q", want, copied)
		}
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if got := tt.Clipboard(); !strings.Contains(got, "**text**") || !strings.Contains(got, "| one | two |") {
		t.Fatalf("Markdown copy lost syntax: %q", got)
	}
}

func TestFirstPresentationIsOnceAndGenerationBound(t *testing.T) {
	var events []diagnostics.Event
	a := &readerWindow{documentSession: documentSession{path: "/tmp/timing.md", source: "# Ready"}}
	prepareTestReader(t, a)
	a.performance = readerPerformance{ctx: diagnostics.WithObserver(context.Background(), func(e diagnostics.Event) { events = append(events, e) }), started: time.Now()}
	tt := ui.NewTester(a.view, 900, 720)
	tt.Frame()
	if len(events) != 1 || events[0].Stage != "native.open-to-first-present" {
		t.Fatalf("presentation events: %+v", events)
	}
	a.loading = true
	a.generation++
	a.performance.first = false
	tt.Frame()
	if len(events) != 1 {
		t.Fatal("loading view counted as document presentation")
	}
	a.loading = false
	a.closed = true
	tt.Frame()
	if len(events) != 1 {
		t.Fatal("closed document counted as presented")
	}
}

func TestPresentationCallbackRunsOncePerFrame(t *testing.T) {
	count := 0
	tt := ui.NewTester(func(c *ui.Context) { ui.Text(c, "Frame"); c.AfterPresent(func() { count++ }) }, 200, 100)
	if count != 1 {
		t.Fatalf("initial frame callback count: %d", count)
	}
	tt.Frame()
	if count != 2 {
		t.Fatalf("next frame callback count: %d", count)
	}
}

// Compare the old source-based API with document adoption using the same block
// styles/layout. This isolates UI work, not preprocessing or GPU presentation.
func BenchmarkNativeFirstFrame(b *testing.B) {
	for _, tc := range nativeRenderCases() {
		for _, prepared := range []bool{false, true} {
			mode := "source"
			if prepared {
				mode = "prepared"
			}
			b.Run(tc.name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					a := benchmarkReader(b, tc.source)
					render := benchmarkView(a, prepared)
					b.StartTimer()
					tt := ui.NewTester(render, 900, 720)
					b.StopTimer()
					if !tt.HasText("Reading") {
						b.Fatal("missing rendered document")
					}
					a.close()
					b.StartTimer()
				}
			})
		}
	}
}

func BenchmarkNativeScroll(b *testing.B) {
	for _, tc := range nativeRenderCases() {
		b.Run(tc.name, func(b *testing.B) {
			a := benchmarkReader(b, tc.source)
			defer a.close()
			tt := ui.NewTester(a.view, 900, 720)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				tt.Scroll(500, 400, 0, 120)
				tt.Scroll(500, 400, 0, -120)
			}
		})
	}
}

func nativeRenderCases() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"prose-1000", "# Reading\n\n" + strings.Repeat("A paragraph with **emphasis** and a [link](https://example.com).\n\n", 1000)},
		{"tables-100", "# Reading\n\n" + strings.Repeat("| Key | Value |\n|---|---|\n| one | two |\n\n", 100)},
		{"aligned-100", "# Reading\n\n" + strings.Repeat("<div align=center>\n\n## Heading\n\nAligned **paragraph**.\n\n</div>\n\n", 100)},
	}
}
func benchmarkReader(b *testing.B, source string) *readerWindow {
	b.Helper()
	prepared, err := prepareReaderDocument(context.Background(), source)
	if err != nil {
		b.Fatal(err)
	}
	return &readerWindow{documentSession: documentSession{path: "/tmp/benchmark.md", source: source, prepared: prepared}}
}
func benchmarkView(a *readerWindow, prepared bool) func(*ui.Context) {
	return func(c *ui.Context) {
		if a.md == nil {
			a.md = &markdown.Options{}
		}
		a.md.Theme, a.md.BlockStyle = readerTheme(c.Theme().Dark), a.readerBlockStyle
		left, prose, _ := readerWidths(c)
		ui.Column(c).Fill().Children(func() {
			a.titleBar(c)
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Width(prose).Margin(0, 0, 0, left).Padding(40, 0, 120, 0).FontWeight(400).Children(func() {
					if prepared {
						markdown.ViewDocument(c, a.prepared.parsed, a.md)
					} else {
						markdown.View(c, a.prepared.markdown, a.md)
					}
				})
			})
		})
	}
}

func TestLongDocumentCopiesBeyondViewport(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/long-copy.md", source: "# Start\n\n" + strings.Repeat("Paragraph with a [link](https://example.com).\n\n", 1200) + "## Far end\n\nLast content."}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	r, _ := tt.Find("Start")
	tt.ClickAt(r.X+5, r.Y+r.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	got := tt.Clipboard()
	if strings.Count(got, "Paragraph with a link.") != 1200 || !strings.Contains(got, "Last content.") {
		t.Fatal("copy omitted offscreen blocks")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if !strings.Contains(tt.Clipboard(), "## Far end") {
		t.Fatal("Markdown copy lost offscreen heading")
	}
}

func TestLargeCodeRemainsSelectableAcrossThemeAndReplacement(t *testing.T) {
	source := "```go\n" + strings.Repeat("var value = 42 // example\n", 1800) + "last_line\n```\n"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/code.md", source: source}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	r, _ := tt.Find("Code, go")
	tt.ClickAt(r.X+15, r.Y+60)
	tt.Command("selectAll")
	tt.Command("copy")
	if !strings.Contains(tt.Clipboard(), "last_line") || strings.Count(tt.Clipboard(), "var value = 42") != 1800 {
		t.Fatal("large plain code lost content")
	}
	tt.SetDark(true)
	tt.Frame()
	a.source = "```go\nsmall_replacement\n```\n"
	prepareTestReader(t, a)
	tt.Frame()
	if !tt.HasText("small_replacement") || tt.HasText("last_line") {
		t.Fatal("code cache retained prior document")
	}
}

func BenchmarkLargeCodeFirstFrame(b *testing.B) {
	source := "```go\n" + strings.Repeat("var value = 42 // example\n", 1800) + "last_line\n```\n"
	for _, limit := range []int{0, readerHighlightLimit} {
		name := "highlighted"
		if limit > 0 {
			name = "bounded"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				prepared, err := prepareReaderDocument(context.Background(), source)
				if err != nil {
					b.Fatal(err)
				}
				options := &markdown.Options{HighlightLimit: limit}
				b.StartTimer()
				tt := ui.NewTester(func(c *ui.Context) { markdown.ViewDocument(c, prepared.parsed, options) }, 900, 720)
				b.StopTimer()
				if !tt.HasText("last_line") {
					b.Fatal("missing code")
				}
				b.StartTimer()
			}
		})
	}
}
