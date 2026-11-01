package preview

import (
	"context"
	"sync"
	"sync/atomic"
)

type request struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Requests uses opaque integers across the C boundary, never Go pointers.
// Release may race with rendering: a renderer retains only its Go context.
type Requests struct {
	next   atomic.Uint64
	active sync.Map
}

func (r *Requests) New() uint64 {
	id := r.next.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	r.active.Store(id, request{ctx, cancel})
	return id
}
func (r *Requests) Context(id uint64) (context.Context, bool) {
	value, ok := r.active.Load(id)
	if !ok {
		return nil, false
	}
	return value.(request).ctx, true
}
func (r *Requests) Cancel(id uint64) {
	if value, ok := r.active.Load(id); ok {
		value.(request).cancel()
	}
}
func (r *Requests) Release(id uint64) {
	if value, ok := r.active.LoadAndDelete(id); ok {
		value.(request).cancel()
	}
}
