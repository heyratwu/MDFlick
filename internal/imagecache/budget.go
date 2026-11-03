package imagecache

import "sync"

// ByteBudget accounts for retained cache bytes across documents. It estimates
// retained pixels, not process/GPU RSS. A ByteBudget must not be copied.
type ByteBudget struct {
	mu          sync.Mutex
	used, limit int64
}

func NewByteBudget(limit int64) *ByteBudget { return &ByteBudget{limit: limit} }

// TryReserve atomically reserves n bytes, rejecting negative or excess charges.
func (b *ByteBudget) TryReserve(n int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n < 0 || n > b.limit-b.used {
		return false
	}
	b.used += n
	return true
}

// Release returns a previous reservation. Callers must return each charge once.
func (b *ByteBudget) Release(n int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.releaseLocked(n)
}

func (b *ByteBudget) releaseLocked(n int64) { b.used -= n }

func (b *ByteBudget) Used() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// Available is a snapshot for eviction decisions, not a reservation. Admission
// must still use TryReserve because another document may consume these bytes.
func (b *ByteBudget) Available() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit - b.used
}
