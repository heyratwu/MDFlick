package main

import (
	"context"
	"sync"
)

// Owned by application, shared by its windows. Slots bound active work; callers
// waiting for a slot retain only their input path/job and cancellation context.
type applicationResources struct {
	loads, diagrams chan struct{}
	images          *imageResources
	diagramCache    *byteBudget
}

func newApplicationResources() *applicationResources {
	return &applicationResources{
		loads: make(chan struct{}, 2), diagrams: make(chan struct{}, 2),
		images:       &imageResources{limit: applicationImageBudget, slots: make(chan struct{}, 4)},
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
