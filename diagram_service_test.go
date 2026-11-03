package main

import (
	"testing"

	"mdflick/internal/imagecache"
)

type fakeDiagramBackend struct {
	callbacks        []func(diagramResult)
	rejected, closed bool
}

func (f *fakeDiagramBackend) submit(_ diagramKey, done func(diagramResult)) bool {
	if f.rejected {
		return false
	}
	f.callbacks = append(f.callbacks, done)
	return true
}
func (f *fakeDiagramBackend) close() { f.closed = true }

func TestDiagramServiceLifetimeAndAdmission(t *testing.T) {
	backend := &fakeDiagramBackend{}
	var publications []func()
	s := newDiagramService(backend, func(fn func()) { publications = append(publications, fn) }, imagecache.NewByteBudget(128<<20))
	key := diagramKey{code: "one"}
	s.request(key)
	s.request(key)
	if len(backend.callbacks) != 1 {
		t.Fatal("duplicate work submitted")
	}
	backend.callbacks[0](diagramResult{bytes: 4})
	if s.entries[key] != nil {
		t.Fatal("worker published without UI dispatch")
	}
	s.close()
	s.close()
	current := newDiagramService(&fakeDiagramBackend{}, func(fn func()) { fn() }, imagecache.NewByteBudget(128<<20))
	publications[0]()
	if !backend.closed || s.bytes != 0 || len(s.entries) != 0 || len(current.entries) != 0 {
		t.Fatal("closed service retained or published results")
	}
	if _, ok := s.request(key); ok {
		t.Fatal("closed service accepted work")
	}
	current.close()
}

func TestDiagramServiceBudgetAndQueueFailure(t *testing.T) {
	backend := &fakeDiagramBackend{}
	s := newDiagramService(backend, func(fn func()) { fn() }, imagecache.NewByteBudget(128<<20))
	defer s.close()
	one, two := diagramKey{code: "one"}, diagramKey{code: "two"}
	s.request(one)
	backend.callbacks[0](diagramResult{bytes: diagramCacheBudget})
	backend.callbacks[0](diagramResult{bytes: 4})
	s.request(two)
	backend.callbacks[1](diagramResult{bytes: 4})
	if s.bytes != diagramCacheBudget || s.entries[two].err == nil || s.entries[two].image != nil {
		t.Fatal("cache budget or duplicate result handling failed")
	}
	backend.rejected = true
	failed := diagramKey{code: "rejected"}
	result, ok := s.request(failed)
	if ok || result != nil {
		t.Fatal("full queue admitted work")
	}
	backend.rejected = false
	if _, ok = s.request(failed); !ok || len(backend.callbacks) != 3 {
		t.Fatal("temporary rejection permanently cached")
	}
	for i := len(s.entries); i < 128; i++ {
		s.entries[diagramKey{code: string(rune(i + 1000))}] = nil
	}
	if _, ok := s.request(diagramKey{code: "overflow"}); ok {
		t.Fatal("entry limit ignored")
	}
}

func TestDiagramViewportRejectsCancelledPublication(t *testing.T) {
	backend := &fakeDiagramBackend{}
	s := newDiagramService(backend, func(fn func()) { fn() }, imagecache.NewByteBudget(128<<20))
	defer s.close()
	key := diagramKey{code: "returning"}
	s.update(map[diagramKey]float32{key: 0})
	old := backend.callbacks[0]
	s.update(map[diagramKey]float32{})
	s.update(map[diagramKey]float32{key: 0})
	old(diagramResult{bytes: 100})
	if s.entries[key] != nil || s.bytes != 0 {
		t.Fatal("cancelled generation restored result")
	}
	backend.callbacks[1](diagramResult{bytes: 20})
	if s.entries[key] == nil || s.bytes != 20 {
		t.Fatal("new generation lost")
	}
}

func TestDiagramViewportReclaimsForVisibleResult(t *testing.T) {
	backend := &fakeDiagramBackend{}
	budget := imagecache.NewByteBudget(diagramCacheBudget)
	s := newDiagramService(backend, func(fn func()) { fn() }, budget)
	defer s.close()
	one, two := diagramKey{code: "one"}, diagramKey{code: "two"}
	s.update(map[diagramKey]float32{one: 0})
	backend.callbacks[0](diagramResult{bytes: diagramCacheBudget})
	s.update(map[diagramKey]float32{two: 0})
	backend.callbacks[1](diagramResult{bytes: 1024})
	if s.entries[one] != nil || s.entries[two].err != nil || s.bytes != 1024 || budget.Used() != 1024 {
		t.Fatal("offscreen result pinned budget", s.bytes, budget.Used())
	}
	s.update(map[diagramKey]float32{one: 0})
	if len(backend.callbacks) != 3 {
		t.Fatal("evicted result could not return")
	}
}

func TestDiagramReservationReclaimsWarmOffscreenResult(t *testing.T) {
	backend := &fakeDiagramBackend{}
	budget := imagecache.NewByteBudget(10)
	s := newDiagramService(backend, func(fn func()) { fn() }, budget)
	defer s.close()
	one, two := diagramKey{code: "one"}, diagramKey{code: "two"}
	s.update(map[diagramKey]float32{one: 0})
	backend.callbacks[0](diagramResult{bytes: 4})
	s.update(map[diagramKey]float32{two: 0})
	if s.entries[one] == nil {
		t.Fatal("warm result reclaimed before budget pressure")
	}
	backend.callbacks[1](diagramResult{bytes: 8})
	if _, exists := s.entries[one]; exists || s.entries[two].err != nil || budget.Used() != 8 {
		t.Fatal("hard reservation failed to reclaim warm offscreen result")
	}
	if _, exists := s.tickets[one]; exists {
		t.Fatal("eviction retained publication ticket")
	}

	// A visible result cannot be reclaimed to make room for another visible key.
	s.update(map[diagramKey]float32{one: 0, two: 0})
	backend.callbacks[2](diagramResult{bytes: 4})
	if s.entries[one].err != errDiagramBudget || s.entries[two].bytes != 8 || budget.Used() != 8 {
		t.Fatal("visible result was evicted or failed reservation charged bytes")
	}
}
