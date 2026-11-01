package main

import (
	"context"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestImagesReloadAndCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	write := func(c color.RGBA, width int) {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		im := image.NewRGBA(image.Rect(0, 0, width, 1))
		im.Set(0, 0, c)
		err = png.Encode(f, im)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	write(color.RGBA{R: 255, A: 255}, 1)
	a := &readerWindow{}
	a.resetMarkdown(context.Background(), filepath.Join(dir, "doc.md"))
	options := a.md
	first, err := a.md.LoadImage("image.png")
	if err != nil {
		t.Fatal(err)
	}
	write(color.RGBA{B: 255, A: 255}, 2)
	a.resetMarkdown(context.Background(), filepath.Join(dir, "doc.md"))
	if a.md == options {
		t.Fatal("reload retained Options image cache")
	}
	second, err := a.md.LoadImage("image.png")
	if err != nil {
		t.Fatal(err)
	}
	w1, _ := first.(*ui.Bitmap).Size()
	w2, _ := second.(*ui.Bitmap).Size()
	if w1 != 1 || w2 != 2 {
		t.Fatal("reload reused old image dimensions")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := localImages(ctx, dir)("image.png"); err != context.Canceled {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(maxImageBytes + 1)
	f.Close()
	if _, err := localImages(context.Background(), dir)("image.png"); err == nil {
		t.Fatal("oversized image accepted")
	}
}
