package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if _, err := documentImages(ctx, dir)("image.png"); err != context.Canceled {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(maxImageBytes + 1)
	f.Close()
	if _, err := documentImages(context.Background(), dir)("image.png"); err == nil {
		t.Fatal("oversized image accepted")
	}
}

func TestRemoteImages(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/photo":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData.Bytes())
		case "/redirect":
			http.Redirect(w, r, "/photo?raw=1", http.StatusFound)
		case "/badge":
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="8"><rect width="12" height="8" fill="red"/></svg>`)
		case "/large":
			w.Header().Set("Content-Length", strconv.Itoa(maxImageBytes+1))
		case "/large-stream":
			w.(http.Flusher).Flush()
			io.WriteString(w, strings.Repeat("x", maxImageBytes+1))
		case "/large-pixels":
			// A GIF logical screen of 5000 x 5000 exceeds the pixel budget.
			w.Write([]byte{'G', 'I', 'F', '8', '9', 'a', 0x88, 0x13, 0x88, 0x13, 0, 0, 0})
		case "/html":
			io.WriteString(w, "<html>sign in required</html>")
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/file-redirect":
			http.Redirect(w, r, "file:///tmp/image.png", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	// Exercise the loader wired into the document's Markdown options.
	a := &readerWindow{}
	a.resetMarkdown(context.Background(), filepath.Join(t.TempDir(), "doc.md"))
	for _, path := range []string{"/photo?raw=1", "/redirect"} {
		src, err := a.md.LoadImage(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		w, h := src.(*ui.Bitmap).Size()
		if w != 3 || h != 2 {
			t.Fatalf("unexpected bitmap size %dx%d", w, h)
		}
	}
	if src, err := a.md.LoadImage(server.URL + "/badge"); err != nil {
		t.Fatal(err)
	} else if _, ok := src.(*ui.SVG); !ok {
		t.Fatalf("expected SVG, got %T", src)
	}
	for _, path := range []string{"/missing", "/large", "/large-stream", "/large-pixels", "/html", "/loop", "/file-redirect"} {
		t.Run(path, func(t *testing.T) {
			if _, err := a.md.LoadImage(server.URL + path); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	for _, address := range []string{"ftp://example.com/image.png", "data:image/png;base64,AAAA"} {
		if _, err := a.md.LoadImage(address); err == nil {
			t.Fatalf("unsupported address accepted: %s", address)
		}
	}
}

func TestRemoteImageCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := documentImages(ctx, t.TempDir())(server.URL + "/slow.png")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop request")
	}
}
