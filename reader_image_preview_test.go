package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestImagePreviewLayout(t *testing.T) {
	for _, size := range []image.Point{{80, 60}, {1600, 400}, {400, 1600}, {1600, 1600}} {
		a := &readerWindow{documentSession: documentSession{path: "/tmp/photo.png"}, preview: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, size.X, size.Y)))}
		a.sidebar = readerSidebar{open: true, directory: "/tmp"}
		tt := ui.NewTester(a.view, 900, 720)
		tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
		for _, window := range []image.Point{{900, 720}, {480, 360}, {1600, 1000}} {
			tt.SetSize(window.X, window.Y)
			r, ok := tt.Find("图片预览")
			if !ok {
				t.Fatal("missing image preview")
			}
			left := min(float32(240), float32(window.X)*.35)
			w, h := previewImageSize(float32(size.X), float32(size.Y), float32(window.X)-left-48, float32(window.Y)-40-48)
			if r.W != w || r.H != h || r.X < left+24 || r.Y < 64 || r.X+r.W > float32(window.X)-24 || r.Y+r.H > float32(window.Y)-24 {
				t.Fatalf("image %v window %v: bounds %+v, want %gx%g", size, window, r, w, h)
			}
			if _, ok := tt.Find("复制 Markdown 全文"); ok {
				t.Fatal("image has Markdown copy action")
			}
		}
	}
}

func TestWelcomeHidesSidebarToggle(t *testing.T) {
	a := &readerWindow{}
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
	for i := 0; i < 3; i++ {
		if _, ok := tt.Find("切换侧边栏"); ok {
			t.Fatal("welcome has sidebar toggle")
		}
		a.path = "/tmp/photo.jpg"
		tt.Frame()
		if _, ok := tt.Find("切换侧边栏"); !ok {
			t.Fatal("opened file has no sidebar toggle")
		}
		a.path = ""
		tt.Frame()
	}
}

func TestImagePreviewSnapshot(t *testing.T) {
	output := os.Getenv("MDFLICK_IMAGE_SNAPSHOT")
	if output == "" {
		t.Skip("optional rendered snapshot")
	}
	img := image.NewRGBA(image.Rect(0, 0, 1400, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1400; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / 1400), uint8(y * 255 / 900), 128, 255})
		}
	}
	a := &readerWindow{documentSession: documentSession{path: "/tmp/preview/photo.png"}, preview: ui.NewBitmap(img)}
	a.sidebar = readerSidebar{open: true, directory: "/tmp/preview", entries: []sidebarEntry{{name: "photos", path: "/tmp/preview/photos", directory: true}, {name: "photo.png", path: a.path, enabled: true}, {name: "readme.md", path: "/tmp/preview/readme.md", enabled: true}}}
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestSidebarImageFormats(t *testing.T) {
	for _, ext := range []string{"png", "JPEG", "jpg", "bmp", "gif", "webp", "svg", "heic", "tiff", "avif"} {
		path := "/tmp/photo." + ext
		if !isImagePath(path) {
			t.Fatalf("missing image icon: %s", ext)
		}
	}
	manager := &application{windows: map[string]*readerWindow{}}
	a := &readerWindow{}
	for _, ext := range []string{"png", "JPEG", "jpg", "bmp", "gif", "webp"} {
		path := "/tmp/photo." + ext
		if !manager.replaceWindowPath(a, path) || a.path != path {
			t.Fatalf("cannot select %s", path)
		}
	}
}
