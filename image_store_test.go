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
)

// Shared fixture; production resources belong to application.
var applicationImages = &imageResources{limit: applicationImageBudget, slots: make(chan struct{}, 4)}

func imageTestResources(limit int64, concurrency int) *imageResources {
	return &imageResources{limit: limit, slots: make(chan struct{}, concurrency)}
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
	defer s.close()
	results := make(chan ui.ImageSource, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { im, err := s.load(server.URL + "/image#ignored"); results <- im; errs <- err }()
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
	im, err := s.load(server.URL + "/image#another")
	if err != nil || im != first || requests.Load() != 1 || s.used != 24+int64(data.Len()) {
		t.Fatal("cache reuse or accounting failed", err, requests.Load(), s.used)
	}
}

func TestImageStoreBudgetsAndRelease(t *testing.T) {
	dir := t.TempDir()
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	for _, name := range []string{"one.png", "two.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	charge := int64(data.Len()) + 16
	resources := imageTestResources(charge*2, 2)
	makeStore := func() *imageStore {
		s := newImageStore(context.Background(), filepath.Join(dir, "doc.md"), resources)
		t.Cleanup(s.close)
		return s
	}
	first, second, third := makeStore(), makeStore(), makeStore()
	first.limit = charge
	a, err := first.load("one.png")
	if err != nil {
		t.Fatal(err)
	}
	alias, err := first.load("./one.png")
	if err != nil || alias != a {
		t.Fatal("local alias not shared")
	}
	if _, err := first.load("two.png"); !errors.Is(err, errImageBudget) {
		t.Fatal("document budget ignored", err)
	}
	if _, err := second.load("one.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := third.load("one.png"); !errors.Is(err, errImageBudget) {
		t.Fatal("application budget ignored", err)
	}
	first.close()
	first.close()
	if resources.used != charge {
		t.Fatal("release was not idempotent", resources.used)
	}
	// A different resource can immediately use the budget released by the
	// closed document; the failed address retains its retry cooldown.
	if _, err := third.load("two.png"); err != nil {
		t.Fatal("released budget unusable", err)
	}
	third.maxEntries = len(third.entries)
	if _, err := third.load("missing.png"); !errors.Is(err, errImageBudget) {
		t.Fatal("entry limit ignored", err)
	}
	if _, err := first.load("one.png"); !errors.Is(err, context.Canceled) {
		t.Fatal("closed store accepted load", err)
	}
	second.close()
	third.close()
	if resources.used != 0 {
		t.Fatal("budget leaked", resources.used)
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
	defer s.close()
	if _, err := s.load("bad.png"); err == nil {
		t.Fatal("corrupt bitmap accepted")
	}
	if s.used != 0 || resources.used != 0 {
		t.Fatal("failed decode retained reservation")
	}
}

func TestImageStoreGlobalConcurrencyAndCancel(t *testing.T) {
	resources := imageTestResources(1024, 1)
	started := make(chan struct{}, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	first := newImageStore(context.Background(), "/tmp/a.md", resources)
	second := newImageStore(context.Background(), "/tmp/b.md", resources)
	defer first.close()
	defer second.close()
	result := make(chan error, 3)
	go func() { _, err := first.load(server.URL + "/one"); result <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not start")
	}
	// Duplicate waiters and another document must all unblock on cancellation.
	go func() { _, err := first.load(server.URL + "/one"); result <- err }()
	go func() { _, err := second.load(server.URL + "/two"); result <- err }()
	select {
	case <-started:
		t.Fatal("global concurrency exceeded")
	case <-time.After(50 * time.Millisecond):
	}
	second.close()
	first.close()
	for i := 0; i < 3; i++ {
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancellation stalled")
		}
	}
	if len(resources.slots) != 0 || resources.used != 0 {
		t.Fatal("cancelled work retained resources")
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
	if _, err := firstStore.load(server.URL + "/image"); !errors.Is(err, context.Canceled) {
		t.Fatal("old document loader remained active", err)
	}
}

func TestImageViewportReclaimsAndReloads(t *testing.T) {
	dir := t.TempDir()
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	for _, name := range []string{"one.png", "two.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	charge := int64(data.Len()) + 16
	resources := imageTestResources(charge, 2)
	s := newImageStore(context.Background(), filepath.Join(dir, "doc.md"), resources)
	defer s.close()
	s.limit = charge
	one, _ := s.key("one.png")
	two, _ := s.key("two.png")
	s.updateViewport(map[string]float32{one: 0})
	if _, err := s.load(one); err != nil {
		t.Fatal(err)
	}
	s.updateViewport(map[string]float32{two: 0})
	if _, err := s.load(two); err != nil {
		t.Fatal("offscreen bitmap prevented visible image", err)
	}
	if _, ok := s.peek(one); ok {
		t.Fatal("old bitmap retained")
	}
	s.updateViewport(map[string]float32{one: 0})
	if _, err := s.load(one); err != nil {
		t.Fatal("evicted image failed to return", err)
	}
	if s.used != charge || resources.used != charge {
		t.Fatal("eviction accounting drift", s.used, resources.used)
	}
	s.close()
	if resources.used != 0 {
		t.Fatal("close leaked reclaimed budget")
	}
}

func TestTemporaryImageFailureCanRetryAfterCooldown(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	}))
	defer server.Close()
	s := newImageStore(context.Background(), "/tmp/doc.md", imageTestResources(1024, 2))
	defer s.close()
	if _, err := s.load(server.URL); !errors.Is(err, errImageTemporary) {
		t.Fatal(err)
	}
	s.load(server.URL)
	if requests.Load() != 1 {
		t.Fatal("failure retried on every frame")
	}
	s.mu.Lock()
	s.entries[server.URL].retry = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err := s.load(server.URL); err != nil || requests.Load() != 2 {
		t.Fatal("temporary failure became permanent", err)
	}
}
