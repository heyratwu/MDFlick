package main

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/document"
)

func TestReaderHTMLLinkedPicture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		img := image.NewRGBA(image.Rect(0, 0, 800, 240))
		col := color.RGBA{R: 40, G: 130, B: 190, A: 255}
		if r.URL.Path == "/dark" {
			col = color.RGBA{R: 230, G: 160, B: 40, A: 255}
		}
		for y := 0; y < 240; y++ {
			for x := 0; x < 800; x++ {
				img.SetRGBA(x, y, col)
			}
		}
		png.Encode(w, img)
	}))
	defer server.Close()
	source := `Before the heading.

<h1 align="center">
  <br>
  <a href="https://example.com/project">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="` + server.URL + `/dark">
      <source media="(prefers-color-scheme: light)" srcset="` + server.URL + `/light">
      <img src="` + server.URL + `/fallback" alt="Project logo" width="400">
    </picture>
  </a>
  <br>
  <a href="https://example.com/program"><img src="` + server.URL + `/badge" alt="Program badge" width="160" height="48"></a>
</h1>

Outside the heading.`
	a := &readerWindow{documentSession: documentSession{path: "/tmp/html-images.md", source: source}}
	a.resetMarkdown(context.Background(), a.path)
	prepareTestReader(t, a)
	clicked := ""
	a.onLink = func(dest string) { clicked = dest }
	tt := ui.NewTester(a.view, 1000, 700)
	for _, theme := range []struct {
		dark bool
		path string
	}{{false, "/light"}, {true, "/dark"}, {false, "/light"}} {
		tt.SetDark(theme.dark)
		for _, path := range []string{theme.path, "/badge"} {
			if _, err := a.images.load(server.URL + path); err != nil {
				t.Fatal(err)
			}
		}
		tt.Frame()
		logo, ok := tt.Find("Project logo")
		if !ok || abs(logo.W-400) > 1 || abs(logo.H-120) > 1 || abs(logo.X+logo.W/2-500) > 1 {
			t.Fatalf("logo bounds: %+v, texts=%q", logo, tt.Texts())
		}
		badge, ok := tt.Find("Program badge")
		if !ok || badge.Y < logo.Y+logo.H || abs(badge.X+badge.W/2-500) > 1 {
			t.Fatalf("badge bounds: %+v; logo=%+v", badge, logo)
		}
		a.imageScheduler.mu.Lock()
		_, wanted := a.imageScheduler.wanted[server.URL+theme.path]
		a.imageScheduler.mu.Unlock()
		if !wanted {
			t.Fatalf("theme image %s was not scheduled", theme.path)
		}
		if err := tt.Click("Project logo"); err != nil {
			t.Fatal(err)
		}
		if clicked != "https://example.com/project" {
			t.Fatal(clicked)
		}
		if err := tt.Click("Program badge"); err != nil {
			t.Fatal(err)
		}
		if clicked != "https://example.com/program" {
			t.Fatal(clicked)
		}
	}
	tt.SetSize(450, 700)
	logo, _ := tt.Find("Project logo")
	if logo.W > 306.5 || logo.X < 0 || logo.X+logo.W > 450 {
		t.Fatalf("narrow image: %+v", logo)
	}
	// Unified copy still uses the original document's inlines and all text.
	text, _ := tt.Find("Outside the heading.")
	tt.ClickAt(text.X+4, text.Y+text.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	copied := tt.Clipboard()
	for _, want := range []string{"Before the heading.", "Outside the heading."} {
		if !strings.Contains(copied, want) {
			t.Fatalf("copy lost %q: %q", want, copied)
		}
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	for _, want := range []string{"![Project logo]", "![Program badge]", "Outside the heading."} {
		if !strings.Contains(tt.Clipboard(), want) {
			t.Fatalf("Markdown copy lost %q: %q", want, tt.Clipboard())
		}
	}
	if path := os.Getenv("MDFLICK_HTML_IMAGES_SCREENSHOT"); path != "" {
		tt.SetSize(1000, 700)
		tt.SetDark(false)
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestReaderMarkdownLinkedAndHeadingImages(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/md-images.md", source: "# ![Heading](heading.png)\n\n[![Linked](linked.png)](https://example.com)\n\n![First](one.png) ![Second](two.png)"}}
	prepareTestReader(t, a)
	calls := map[string]int{}
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 120, 60)))
	a.md = &markdown.Options{ResolveImage: func(c *ui.Context, source string, box ui.Element) ui.ImageSource { calls[source]++; return bitmap }}
	clicked := ""
	a.onLink = func(dest string) { clicked = dest }
	tt := ui.NewTester(a.view, 900, 700)
	for _, source := range []string{"heading.png", "linked.png", "one.png", "two.png"} {
		if calls[source] == 0 {
			t.Fatalf("image %s not resolved: %+v", source, calls)
		}
	}
	for _, alt := range []string{"Heading", "Linked", "First", "Second"} {
		r, ok := tt.Find(alt)
		if !ok || r.W < 119 || r.H < 59 {
			t.Fatalf("image %s: %+v", alt, r)
		}
	}
	if err := tt.Click("Linked"); err != nil {
		t.Fatal(err)
	}
	if clicked != "https://example.com" {
		t.Fatal(clicked)
	}
	a.source = "# Replaced\n\nPlain text"
	prepareTestReader(t, a)
	tt.Frame()
	if tt.HasText("Linked") || !tt.HasText("Replaced") {
		t.Fatal(tt.Texts())
	}

}

func TestNativeImageSize(t *testing.T) {
	for _, tc := range []struct {
		im                      document.HTMLImage
		nw, nh, available, w, h float32
	}{
		{document.HTMLImage{Width: 400}, 800, 240, 820, 400, 120},
		{document.HTMLImage{Height: 120}, 800, 240, 820, 400, 120},
		{document.HTMLImage{Width: 400, Height: 200}, 800, 240, 300, 300, 150},
		{document.HTMLImage{WidthPercent: 50}, 800, 240, 600, 300, 90},
		{document.HTMLImage{}, 800, 240, 400, 400, 120},
	} {
		w, h := nativeImageSize(tc.im, tc.nw, tc.nh, tc.available)
		if abs(w-tc.w) > .01 || abs(h-tc.h) > .01 {
			t.Fatalf("%+v => %v,%v", tc, w, h)
		}
	}
}
