package main

import "testing"

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
	s := newDiagramService(backend, func(fn func()) { publications = append(publications, fn) }, &byteBudget{limit: 128 << 20})
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
	current := newDiagramService(&fakeDiagramBackend{}, func(fn func()) { fn() }, &byteBudget{limit: 128 << 20})
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
	s := newDiagramService(backend, func(fn func()) { fn() }, &byteBudget{limit: 128 << 20})
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
	if !ok || result == nil || result.err == nil {
		t.Fatal("queue failure not cached")
	}
	for i := len(s.entries); i < 128; i++ {
		s.entries[diagramKey{code: string(rune(i + 1000))}] = nil
	}
	if _, ok := s.request(diagramKey{code: "overflow"}); ok {
		t.Fatal("entry limit ignored")
	}
}
