package main

import (
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReaderHTMLAlignment(t *testing.T) {
	source, err := os.ReadFile("internal/testfixtures/markdown/html.md")
	if err != nil {
		t.Fatal(err)
	}
	a := &readerWindow{documentSession: documentSession{path: "/tmp/html.md", source: string(source)}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1000, 900)
	// Check actual glyph positions, since text element bounds span the column.
	r, ok := tt.Find("Read, watch, listen, and turn what you learn into Obsidian notes.")
	if !ok {
		t.Fatal("missing centered paragraph")
	}
	img := tt.Image()
	left, right := img.Bounds().Max.X, 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			red, green, blue, _ := img.At(x, y).RGBA()
			if red < 32000 && green < 32000 && blue < 32000 {
				left = min(left, x)
				right = max(right, x)
			}
		}
	}
	if right <= left || math.Abs(float64(left+right)/2-500) > 2 {
		t.Fatalf("paragraph glyphs not centered: %d..%d", left, right)
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		for _, want := range []string{"乔木剪藏 · Qiaomu Clipper", "中文", "English", "Outside the centered container."} {
			if !tt.HasText(want) {
				t.Errorf("missing %q: %v", want, tt.Texts())
			}
		}
		for _, text := range tt.Texts() {
			if strings.Contains(text, "<div") || strings.Contains(text, "</div>") {
				t.Fatal("raw container visible", text)
			}
		}
	}
	if path := os.Getenv("MDFLICK_HTML_SCREENSHOT"); path != "" {
		tt.SetDark(false)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReaderAlignedLinksAndReload(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/html.md", source: "<div align=center>\n\n[Go][ref] [English](#destination)\n\n</div>\n\n" + strings.Repeat("Paragraph.\n\n", 35) + "# Destination\n\n[ref]: https://example.com\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1000, 700)
	clicked := ""
	a.onLink = func(dest string) { clicked = dest }
	tt.Frame()
	if err := tt.Click("Go"); err != nil {
		t.Fatal(err)
	}
	if clicked != "https://example.com" {
		t.Fatalf("reference link resolved to %q", clicked)
	}
	a.onLink = nil
	tt.Frame()
	before, _ := tt.Find("Destination")
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	after, ok := tt.Find("Destination")
	if !ok || before.Y <= 700 || after.Y+after.H > 700.5 || after.Y < 0 || a.html.scrollTo != "" {
		t.Fatalf("anchor scroll: before %+v, after %+v, pending %q", before, after, a.html.scrollTo)
	}
	a.source = "# Reloaded\n\nPlain text\n"
	prepareTestReader(t, a)
	tt.Frame()
	if !tt.HasText("Reloaded") || tt.HasText("Go") || len(a.prepared.ranges) != 0 {
		t.Fatal("stale converted document", tt.Texts())
	}
}
