package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/imagecache"
)

func imageEvent[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Minute):
		t.Fatal("image work stalled")
		var zero T
		return zero
	}
}

func TestReaderImagesWaitForCommittedViewport(t *testing.T) {
	started := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 240, 120)))
	}))
	defer server.Close()
	source := "![first](" + server.URL + "/first)\n\n" + strings.Repeat("A paragraph to keep the distant image offscreen.\n\n", 60) + "<div align=center>\n\n![last](" + server.URL + "/last)\n\n</div>"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/viewport.md", source: source}}
	a.resetMarkdown(context.Background(), a.path)
	defer a.releaseImages()
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 500)
	if got := imageEvent(t, started); got != "/first" {
		t.Fatal("loaded offscreen image", got)
	}
	firstKey := server.URL + "/first"
	// The HTTP handler runs only after admission, so this joins the existing load.
	if _, err := a.images.Load(firstKey); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	distantAdmitted := a.images.Contains(server.URL + "/last")
	if distantAdmitted {
		t.Fatal("offscreen image allocated cache entry")
	}
	a.imageScheduler.mu.Lock()
	_, distantWanted := a.imageScheduler.wanted[server.URL+"/last"]
	a.imageScheduler.mu.Unlock()
	if distantWanted {
		t.Fatal("offscreen image entered pending work")
	}
	first, ok := tt.Find("first")
	if !ok || first.W < 239 || first.H < 119 {
		t.Fatal("loaded image lost its dimensions", first)
	}
	// Exercise both disappearance and return using the actual reader scroll view.
	a.scroll.Y = math.MaxFloat32
	tt.Frame()
	if got := imageEvent(t, started); got != "/last" {
		t.Fatal("scroll did not admit aligned image", got)
	}
	if _, err := a.images.Load(server.URL + "/last"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.scroll.Y = 0
	tt.Frame()
	tt.SetSize(1000, 600)
	if len(started) != 0 {
		t.Fatal("returning or resizing fetched a cached image again")
	}
	if path := os.Getenv("MDFLICK_TEST_IMAGE_SCREENSHOT"); path != "" {
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

func TestImageSchedulerPrioritizesVisibleAndCancelsObsolete(t *testing.T) {
	started, cancelled := make(chan string, 16), make(chan string, 16)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		if r.URL.Path == "/z-visible" && attempts.Add(1) > 1 {
			png.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)))
			return
		}
		<-r.Context().Done()
		cancelled <- r.URL.Path
	}))
	defer server.Close()
	store := newImageStore(context.Background(), "/tmp/scheduler.md", imageTestResources(4096, 2))
	defer store.Close()
	scheduler := newImageScheduler(store)
	changed := make(chan struct{}, 16)
	invalidate := func() { changed <- struct{}{} }
	visible := server.URL + "/z-visible"
	scheduler.update(map[string]float32{server.URL + "/a-prefetch": 100, visible: 0, server.URL + "/y-visible": 0}, invalidate)
	for range 2 {
		if got := imageEvent(t, started); got == "/a-prefetch" {
			t.Fatal("prefetch ran ahead of visible image")
		}
	}
	scheduler.update(map[string]float32{}, invalidate)
	for range 2 {
		imageEvent(t, cancelled)
		imageEvent(t, changed)
	}
	entries, used := store.Stats()
	if entries != 0 || used != 0 {
		t.Fatal("cancelled images retained entries/budget", entries, used)
	}
	// Cancellation is retryable; the queued prefetch URL must never have started.
	scheduler.update(map[string]float32{visible: 0}, invalidate)
	if got := imageEvent(t, started); got != "/z-visible" {
		t.Fatal("obsolete queued work was dispatched", got)
	}
	imageEvent(t, changed)
	if im, ready := store.Peek(visible); !ready || im == nil {
		t.Fatal("cancelled image did not retry")
	}
}

func TestImageSchedulerCancelsApplicationSlotWait(t *testing.T) {
	resources := imageTestResources(4096, 1)
	started := make(chan struct{})
	owner := imagecache.New(context.Background(), "/tmp/owner.md", resources, imagecache.DefaultConfig(), func(ctx context.Context, _, _ string, _ func(int64) error) (int, error) {
		close(started)
		<-ctx.Done()
		return 0, ctx.Err()
	})
	done := make(chan error, 1)
	go func() { _, err := owner.Load("owned.png"); done <- err }()
	imageEvent(t, started)
	defer owner.Close()

	store := newImageStore(context.Background(), "/tmp/slot.md", resources)
	defer store.Close()
	scheduler := newImageScheduler(store)
	changed := make(chan struct{}, 2)
	scheduler.update(map[string]float32{"/tmp/not-read.png": 0}, func() { changed <- struct{}{} })
	// Removing a queued or active request must not consume the other document's
	// slot. If not yet dispatched, there is nothing to cancel or wait for.
	scheduler.update(map[string]float32{}, func() { changed <- struct{}{} })
	scheduler.mu.Lock()
	active := len(scheduler.active)
	scheduler.mu.Unlock()
	if active > 0 {
		imageEvent(t, changed)
	}
	select {
	case err := <-done:
		t.Fatal("cancellation interrupted another document", err)
	default:
	}
	owner.Close()
	if err := imageEvent(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestImageViewportBoundsAndCollectionLimit(t *testing.T) {
	for _, tc := range []struct {
		rect     ui.Rect
		distance float32
		near     bool
	}{
		{ui.Rect{X: 10, Y: 60, W: 100, H: 40}, 0, true},
		{ui.Rect{X: 10, Y: 650, W: 100, H: 40}, 150, true},
		{ui.Rect{X: 10, Y: 701, W: 100, H: 40}, 201, false},
		{ui.Rect{X: 10, Y: -210, W: 100, H: 40}, 210, false},
		{ui.Rect{X: 901, Y: 60, W: 100, H: 40}, 0, false},
	} {
		d, near := imageViewportDistance(tc.rect, 900, 500)
		if d != tc.distance || near != tc.near {
			t.Fatal(tc, d, near)
		}
	}
	frame := &readerImageFrame{wanted: make(map[string]float32)}
	for i := 0; i < 1000; i++ {
		frame.add(fmt.Sprint(i), 100)
	}
	frame.add("visible", 0)
	if len(frame.wanted) != documentImageLimit || frame.wanted["visible"] != 0 {
		t.Fatal("collection not bounded or visible URL lost")
	}
	// A duplicate inside the viewport upgrades the priority of its prefetch.
	frame.add("0", 0)
	if d, ok := frame.wanted["0"]; !ok || d != 0 {
		t.Fatal("duplicate priority not upgraded")
	}
}

func TestImageSchedulerWaitsForLayoutAfterDecode(t *testing.T) {
	started := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	}))
	defer server.Close()
	store := newImageStore(context.Background(), "/tmp/batch.md", imageTestResources(4096, 2))
	defer store.Close()
	scheduler := newImageScheduler(store)
	changed := make(chan struct{}, 8)
	invalidate := func() { changed <- struct{}{} }
	// A large image expanding from its alt-text placeholder may push these
	// other initial candidates out of the viewport before the next frame.
	wanted := map[string]float32{}
	for i := 0; i < 8; i++ {
		wanted[fmt.Sprintf("%s/%d", server.URL, i)] = float32(i)
	}
	scheduler.update(wanted, invalidate)
	imageEvent(t, changed)
	scheduler.mu.Lock()
	if !scheduler.awaitLayout {
		t.Fatal("decode did not require fresh layout")
	}
	active := len(scheduler.active)
	scheduler.mu.Unlock()
	if active > 0 {
		imageEvent(t, changed)
	}
	if len(started) > 2 {
		t.Fatal("drained stale visibility queue before next layout", len(started))
	}
	// The next committed frame removes all candidates without dispatching them.
	scheduler.update(map[string]float32{}, invalidate)
	scheduler.mu.Lock()
	pending := len(scheduler.pending)
	scheduler.mu.Unlock()
	if pending != 0 {
		t.Fatal("stale visibility queue retained")
	}
}

func TestEvictedImageKeepsLayoutAndResizes(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/geometry.md", source: "![picture](picture.png)\n\nAfter image."}}
	a.resetMarkdown(context.Background(), a.path)
	defer a.releaseImages()
	prepareTestReader(t, a)
	a.images.Close()
	var loaded atomic.Bool
	a.images = imagecache.New(context.Background(), a.path, imageTestResources(64<<20, 2), imagecache.DefaultConfig(), func(ctx context.Context, _, _ string, reserve func(int64) error) (ui.ImageSource, error) {
		if loaded.Swap(true) {
			return nil, errors.New("evicted test image")
		}
		if err := reserve(17 << 20); err != nil {
			return nil, err
		}
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1200, 600))), nil
	})
	a.imageScheduler = newImageScheduler(a.images)
	a.md.LoadImage = a.images.Load
	if _, err := a.images.Load("picture.png"); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 900, 720)
	before, _ := tt.Find("After image.")
	a.images.UpdateViewport(map[string]float32{})

	tt.Frame()
	after, _ := tt.Find("After image.")
	if math.Abs(float64(before.Y-after.Y)) > 0.5 {
		t.Fatal("eviction collapsed image", before, after)
	}
	tt.SetSize(600, 720)
	smaller, _ := tt.Find("After image.")
	if smaller.Y >= after.Y {
		t.Fatal("placeholder failed to preserve aspect ratio on resize", smaller, after)
	}
}
