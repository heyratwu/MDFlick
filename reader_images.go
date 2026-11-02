package main

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
)

const imagePrefetchMargin float32 = 200

// One frame collects visibility after layout, including scroll/resize/anchor
// changes. Earlier build passes are discarded by AfterPresent. It holds handles,
// never build elements; only the final callback hands URLs to the scheduler.
type readerImageFrame struct {
	scheduler *imageScheduler
	wanted    map[string]float32
}
type imageAnchorKey struct{}
type imageGeometry struct {
	handle        ui.Handle
	width, height float32
}

func (a *readerWindow) beginImageFrame(c *ui.Context) func() {
	if a.imageScheduler == nil {
		return func() {}
	}
	frame := &readerImageFrame{scheduler: a.imageScheduler, wanted: make(map[string]float32)}
	a.md.ResolveImage = frame.resolve
	services := c.Services()
	return func() {
		c.AfterPresent(func() { frame.scheduler.update(frame.wanted, services.Invalidate) })
	}
}

func (f *readerImageFrame) resolve(c *ui.Context, source string, box ui.Element) ui.ImageSource {
	s := f.scheduler.store
	if !document.NativePolicy().AllowsImage(source) {
		return nil
	}
	key, err := s.key(source)
	if err != nil {
		return nil
	}
	geometry := *ui.Local(box, imageAnchorKey{}, func() *imageGeometry { return &imageGeometry{} })
	anchor := &geometry.handle
	box.Bind(anchor)
	image, _ := s.peek(key)
	switch src := image.(type) {
	case *ui.Bitmap:
		w, h := src.Size()
		geometry.width, geometry.height = float32(w), float32(h)
	case *ui.SVG:
		geometry.width, geometry.height = src.Size()
	}
	if image == nil && geometry.width > 0 && geometry.height > 0 {
		box.Width(geometry.width).AspectRatio(geometry.width / geometry.height)
	}
	width, height := c.Size()
	c.AfterPresent(func() {
		bounds := anchor.Bounds(c)
		distance, near := imageViewportDistance(bounds, width, height)
		if near {
			f.add(key, distance)
		}
	})
	return image
}

func imageViewportDistance(b ui.Rect, width, height float32) (float32, bool) {
	if b.W <= 0 || b.H <= 0 || b.X >= width || b.X+b.W <= 0 {
		return 0, false
	}
	distance := max(float32(0), readerTitleBarHeight-(b.Y+b.H), b.Y-height)
	return distance, distance <= imagePrefetchMargin
}

// Keep collection as well as dispatch bounded. Visible URLs have distance zero;
// a nearer occurrence of a duplicate URL takes priority over its distant one.
func (f *readerImageFrame) add(key string, distance float32) {
	if old, ok := f.wanted[key]; ok {
		f.wanted[key] = min(old, distance)
		return
	}
	if len(f.wanted) >= documentImageLimit {
		var worst string
		var farthest float32 = -1
		for key, d := range f.wanted {
			if d > farthest {
				worst, farthest = key, d
			}
		}
		if distance >= farthest {
			return
		}
		delete(f.wanted, worst)
	}
	f.wanted[key] = distance
}

type scheduledImage struct {
	key      string
	distance float32
}
type imageJob struct{ cancel context.CancelFunc }

// Two persistent workers per active document, no goroutine or framework cache
// entry per URL. The pending queue is replaced each committed frame; obsolete
// active jobs are cancelled, including waits on the application loading slots.
type imageScheduler struct {
	store       *imageStore
	mu          sync.Mutex
	started     bool
	awaitLayout bool
	pending     []scheduledImage
	wanted      map[string]float32
	active      map[string]*imageJob
	wake        chan struct{}
	invalidate  func()
}

func newImageScheduler(store *imageStore) *imageScheduler {
	return &imageScheduler{store: store, active: make(map[string]*imageJob), wake: make(chan struct{}, 2)}
}

func (s *imageScheduler) signal() {
	for range 2 {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

func (s *imageScheduler) update(wanted map[string]float32, invalidate func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store.ctx.Err() != nil {
		return
	}
	if s.store.updateViewport(wanted) && invalidate != nil {
		invalidate()
	}
	s.invalidate, s.wanted = invalidate, wanted
	s.awaitLayout = false
	for key, job := range s.active {
		if _, exists := wanted[key]; !exists {
			job.cancel()
		}
	}
	s.pending = s.pending[:0]
	for key, distance := range wanted {
		if _, ready := s.store.peek(key); ready {
			continue
		}
		if s.active[key] == nil && s.store.canAdmit(key) {
			s.pending = append(s.pending, scheduledImage{key, distance})
		}
	}
	s.sortPending()
	if len(s.pending) == 0 {
		return
	}
	if !s.started {
		s.started = true
		for range 2 {
			go s.work()
		}
	}
	s.signal()
}

func (s *imageScheduler) sortPending() {
	sort.Slice(s.pending, func(i, j int) bool {
		if s.pending[i].distance == s.pending[j].distance {
			return s.pending[i].key < s.pending[j].key
		}
		return s.pending[i].distance < s.pending[j].distance
	})
}

func (s *imageScheduler) work() {
	for {
		select {
		case <-s.store.ctx.Done():
			return
		case <-s.wake:
		}
		for {
			s.mu.Lock()
			if s.awaitLayout || len(s.pending) == 0 || s.store.ctx.Err() != nil {
				s.mu.Unlock()
				break
			}
			request := s.pending[0]
			s.pending = s.pending[1:]
			ctx, cancel := context.WithCancel(s.store.ctx)
			job := &imageJob{cancel: cancel}
			s.active[request.key] = job
			s.mu.Unlock()

			_, err := s.store.loadContext(ctx, request.key)
			cancel()
			s.mu.Lock()
			delete(s.active, request.key)
			// A decoded image can move every following placeholder offscreen.
			// Re-evaluate the committed layout before dispatching another batch.
			s.awaitLayout = true
			// A URL can return to the viewport while its cancelled job is unwinding.
			// Retry it with a fresh context; cancellation is never a cached failure.
			if distance, wanted := s.wanted[request.key]; wanted && errors.Is(err, context.Canceled) && s.store.ctx.Err() == nil {
				s.pending = append(s.pending, scheduledImage{request.key, distance})
				s.sortPending()
			}
			invalidate := s.invalidate
			s.mu.Unlock()
			if s.store.ctx.Err() == nil && invalidate != nil {
				invalidate()
			}
		}
	}
}
