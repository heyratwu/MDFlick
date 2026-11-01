package main

import "errors"

const diagramCacheBudget int64 = 64 << 20

// A backend owns native rendering and its serial queue. A service owns the
// document lifetime, admission, deduplication and results. All service methods
// and published mutations run on the UI thread; only backend callbacks may run
// on workers. publish marshals those callbacks onto the UI thread.
type diagramBackend interface {
	submit(diagramKey, func(diagramResult)) bool
	close()
}

type diagramService struct {
	backend diagramBackend
	publish func(func())
	entries map[diagramKey]*diagramResult
	bytes   int64
	closed  bool
}

func newDiagramService(backend diagramBackend, publish func(func())) *diagramService {
	return &diagramService{backend: backend, publish: publish, entries: make(map[diagramKey]*diagramResult)}
}

func (s *diagramService) request(key diagramKey) (*diagramResult, bool) {
	if s == nil || s.closed {
		return nil, false
	}
	if result, exists := s.entries[key]; exists {
		return result, true
	}
	// Do not evict: rebuilding a document would otherwise resubmit every frame.
	if len(s.entries) >= 128 {
		return nil, false
	}
	s.entries[key] = nil
	if !s.backend.submit(key, func(result diagramResult) {
		s.publish(func() { s.commit(key, result) })
	}) {
		s.entries[key] = &diagramResult{err: errors.New("图表渲染队列已满，重新载入可重试")}
	}
	return s.entries[key], true
}

func (s *diagramService) commit(key diagramKey, result diagramResult) {
	pending, exists := s.entries[key]
	if s.closed || !exists || pending != nil {
		return
	}
	if result.bytes < 0 || result.bytes > diagramCacheBudget-s.bytes {
		result = diagramResult{err: errors.New("图表缓存已达内存上限，显示源码")}
	}
	s.bytes += result.bytes
	s.entries[key] = &result
}

func (s *diagramService) close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.backend.close()
	clear(s.entries)
	s.bytes = 0
}
