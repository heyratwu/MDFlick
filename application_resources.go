package main

import (
	"context"
	"log"
	"sync"

	"mdflick/internal/diagnostics"
	"mdflick/internal/imagecache"
)

// Owned by application, shared by its windows. Slots bound active work; callers
// waiting for a slot retain only their input path/job and cancellation context.
type applicationResources struct {
	loads, diagrams chan struct{}
	images          *imageResources
	diagramCache    *byteBudget
	diagramPool     *diagramPool
}

func newApplicationResources() *applicationResources {
	return &applicationResources{
		loads: make(chan struct{}, 2), diagrams: make(chan struct{}, 2),
		images:       imagecache.NewResources(applicationImageBudget, 4),
		diagramCache: &byteBudget{limit: 128 << 20},
	}
}

func acquireWork(ctx context.Context, slots chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-slots
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Cache accounting is independent of UI scheduling, so it can be tested and
// reused across services. It estimates retained pixels, not process/GPU RSS.
type byteBudget struct {
	mu          sync.Mutex
	used, limit int64
}

func (b *byteBudget) reserve(n int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n < 0 || n > b.limit-b.used {
		return false
	}
	b.used += n
	return true
}
func (b *byteBudget) release(n int64) { b.mu.Lock(); b.used -= n; b.mu.Unlock() }

// Logical cache accounting, alongside heap/RSS samples. Decoders, retained UI
// frames and WebKit may still own memory after these reservations are returned.
func (r *applicationResources) record(stage string) {
	if r == nil || !diagnostics.Enabled() {
		return
	}
	images := r.images.Used()
	r.diagramCache.mu.Lock()
	diagrams := r.diagramCache.used
	r.diagramCache.mu.Unlock()
	log.Printf("resources stage=%s image_bytes=%d diagram_bytes=%d active_loads=%d active_diagrams=%d", stage, images, diagrams, len(r.loads), len(r.diagrams))
}
