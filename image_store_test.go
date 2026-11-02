package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/imagecache"
)

var applicationImages = imagecache.NewResources(applicationImageBudget, 4)

func imageTestResources(limit int64, concurrency int) *imageResources {
	return imagecache.NewResources(limit, concurrency)
}

func TestImageStoreDeduplicates(t *testing.T) {
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 2)))
	var requests atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		<-release
		w.Write(data.Bytes())
	}))
	defer server.Close()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	s := newImageStore(context.Background(), "/tmp/doc.md", imageTestResources(1024, 2))
	defer s.Close()
	results := make(chan ui.ImageSource, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { im, err := s.Load(server.URL + "/image#ignored"); results <- im; errs <- err }()
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	releaseOnce.Do(func() { close(release) })
	var first ui.ImageSource
	for i := 0; i < 12; i++ {
		select {
		case im := <-results:
			if first == nil {
				first = im
			}
			if im != first || im == nil {
				t.Fatal("decoded result was not shared")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("deduplicated caller stalled")
		}
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	// A later adapter using the same loader also gets the same bitmap.
	im, err := s.Load(server.URL + "/image#another")
	_, used := s.Stats()
	if err != nil || im != first || requests.Load() != 1 || used != 24+int64(data.Len()) {
		t.Fatal("cache reuse or accounting failed", err, requests.Load(), used)
	}
}

func TestImageStoreDecodeFailureReleasesReservation(t *testing.T) {
	dir := t.TempDir()
	// Valid header and dimensions, truncated before image data.
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	os.WriteFile(filepath.Join(dir, "bad.png"), data.Bytes()[:33], 0600)
	resources := imageTestResources(1024, 1)
	s := newImageStore(context.Background(), filepath.Join(dir, "doc.md"), resources)
	defer s.Close()
	if _, err := s.Load("bad.png"); err == nil {
		t.Fatal("corrupt bitmap accepted")
	}
	_, used := s.Stats()
	if used != 0 || resources.Used() != 0 {
		t.Fatal("failed decode retained reservation")
	}
}

func TestReaderImagesShareStoreAndRefreshNestedBlocks(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		width := int(requests.Add(1))
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, width, 2)))
	}))
	defer server.Close()
	source := "![outer](" + server.URL + "/image)\n\n<div align=center>\n\n![aligned](" + server.URL + "/image)\n\n</div>\n\n| Image |\n|---|\n| ![table](" + server.URL + "/image) |\n"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/images.md", source: source}}
	t.Cleanup(a.releaseImages)
	a.resetMarkdown(context.Background(), a.path)
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1000, 900)
	firstStore := a.images
	first, err := a.md.LoadImage(server.URL + "/image")
	if err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.resetMarkdown(context.Background(), a.path)
	prepareTestReader(t, a)
	tt.Frame()
	second, err := a.md.LoadImage(server.URL + "/image")
	if err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	w1, _ := first.(*ui.Bitmap).Size()
	w2, _ := second.(*ui.Bitmap).Size()
	if requests.Load() != 2 || w1 != 1 || w2 != 2 || a.images == firstStore {
		t.Fatal("nested views reused old images or fetched duplicates", requests.Load(), w1, w2)
	}
	if _, err := firstStore.Load(server.URL + "/image"); !errors.Is(err, context.Canceled) {
		t.Fatal("old document loader remained active", err)
	}
}

func TestRemoteImageTemporaryErrorsReachCache(t *testing.T) {
	for _, status := range []int{429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(status)
			}))
			defer server.Close()
			s := newImageStore(context.Background(), "/tmp/doc.md", imageTestResources(1024, 2))
			defer s.Close()
			for range 2 {
				if _, err := s.Load(server.URL); !errors.Is(err, imagecache.ErrTemporary) {
					t.Fatal("temporary status lost", err)
				}
			}
			if requests.Load() != 1 {
				t.Fatal("temporary status retried before cooldown", requests.Load())
			}
		})
	}
}
