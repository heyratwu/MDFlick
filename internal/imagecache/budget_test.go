package imagecache

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestByteBudgetReservations(t *testing.T) {
	b := NewByteBudget(12)
	if b.TryReserve(-1) || !b.TryReserve(0) || !b.TryReserve(8) || b.TryReserve(5) {
		t.Fatal("invalid reservation admitted or valid reservation rejected")
	}
	if b.Used() != 8 || b.Available() != 4 || !b.TryReserve(4) || b.TryReserve(1) {
		t.Fatal("budget boundary or failed reservation changed accounting")
	}
	b.Release(8)
	if !b.TryReserve(8) {
		t.Fatal("released bytes unavailable")
	}
	b.Release(12)
	if b.Used() != 0 || b.Available() != 12 {
		t.Fatal("reservation leaked")
	}
}

func TestByteBudgetConcurrentReservations(t *testing.T) {
	const limit = 8
	b := NewByteBudget(limit)
	var accepted atomic.Int32
	var attempted, finished sync.WaitGroup
	release := make(chan struct{})
	for range 64 {
		attempted.Add(1)
		finished.Add(1)
		go func() {
			defer finished.Done()
			ok := b.TryReserve(1)
			if ok {
				accepted.Add(1)
			}
			attempted.Done()
			<-release
			if ok {
				b.Release(1)
			}
		}()
	}
	attempted.Wait()
	used, available := b.Used(), b.Available()
	close(release)
	finished.Wait()
	if accepted.Load() != limit || used != limit || available != 0 || b.Used() != 0 {
		t.Fatalf("concurrent accounting: accepted=%d used=%d available=%d final=%d", accepted.Load(), used, available, b.Used())
	}
}
