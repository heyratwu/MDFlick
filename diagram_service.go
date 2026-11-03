package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"mdflick/internal/imagecache"
)

const diagramCacheBudget int64 = 64 << 20
const diagramWarmBudget int64 = 8 << 20

var errDiagramBudget = errors.New("图表缓存已达内存上限，稍后重试")

// A backend owns a document queue and borrows native runtimes. A service owns the
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
	budget  *imagecache.ByteBudget
	closed  bool
	tickets map[diagramKey]uint64
	next    uint64
	wanted  map[diagramKey]float32
	retries map[diagramKey]time.Time
}

func newDiagramService(backend diagramBackend, publish func(func()), budget *imagecache.ByteBudget) *diagramService {
	return &diagramService{backend: backend, publish: publish, budget: budget, entries: make(map[diagramKey]*diagramResult), tickets: make(map[diagramKey]uint64), retries: make(map[diagramKey]time.Time)}
}

func (s *diagramService) request(key diagramKey) (*diagramResult, bool) {
	if s == nil || s.closed {
		return nil, false
	}
	if result, exists := s.entries[key]; exists {
		return result, true
	}
	// Admission is bounded; committed viewport updates reclaim offscreen entries.
	if len(s.entries) >= 128 {
		return nil, false
	}
	s.entries[key] = nil
	s.next++
	ticket := s.next
	s.tickets[key] = ticket
	if !s.backend.submit(key, func(result diagramResult) {
		s.publish(func() {
			if s.tickets[key] == ticket {
				s.commit(key, result)
			}
		})
	}) {
		delete(s.entries, key)
		delete(s.tickets, key)
		return nil, false
	}
	return s.entries[key], true
}

func (s *diagramService) commit(key diagramKey, result diagramResult) {
	pending, exists := s.entries[key]
	if s.closed || !exists || pending != nil {
		return
	}
	if result.err != nil {
		result.image, result.bytes = nil, 0
		if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled) {
			s.retries[key] = time.Now().Add(2 * time.Second)
		}
	}
	if result.bytes > 0 && s.wanted != nil {
		s.evict(result.bytes, false)
	}
	if result.bytes < 0 || result.bytes > diagramCacheBudget-s.bytes || !s.budget.TryReserve(result.bytes) {
		result = diagramResult{err: errDiagramBudget}
		s.retries[key] = time.Now().Add(2 * time.Second)
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
	clear(s.tickets)
	clear(s.retries)
	s.wanted = nil
	s.budget.Release(s.bytes)
	s.bytes = 0
}

// A committed frame replaces obsolete waiting work, then admits nearest keys.
func (s *diagramService) update(wanted map[diagramKey]float32) {
	if s.closed {
		return
	}
	s.wanted = wanted
	var running map[diagramKey]bool
	if backend, ok := s.backend.(interface {
		reconcile(map[diagramKey]float32) map[diagramKey]bool
	}); ok {
		running = backend.reconcile(wanted)
	}
	for key, result := range s.entries {
		if _, keep := wanted[key]; !keep && result == nil && !running[key] {
			delete(s.entries, key)
			delete(s.tickets, key)
		}
	}
	s.evict(0, true)
	keys := make([]diagramKey, 0, len(wanted))
	for key := range wanted {
		if retry, ok := s.retries[key]; ok && !time.Now().Before(retry) {
			delete(s.entries, key)
			delete(s.retries, key)
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if wanted[keys[i]] == wanted[keys[j]] {
			return keys[i].code < keys[j].code
		}
		return wanted[keys[i]] < wanted[keys[j]]
	})
	for _, key := range keys {
		s.request(key)
	}
}

// Eviction happens after layout or before accepting a result, never in paint.
// The view keeps dimensions separately, so dropping a bitmap does not move text.
func (s *diagramService) evict(incoming int64, warm bool) {
	policy := imagecache.EvictionPolicy{WarmBytes: diagramWarmBudget, WarmEntries: 64, ByteLimit: diagramCacheBudget}
	mode := imagecache.ReserveBytes
	if warm {
		mode = imagecache.TrimWarm
	}
	for key, result := range s.entries {
		if _, visible := s.wanted[key]; visible || result == nil {
			continue
		}
		if !policy.NeedsEviction(mode, s.bytes, len(s.entries), incoming, s.budget.Available()) {
			break
		}
		s.bytes -= result.bytes
		s.budget.Release(result.bytes)
		delete(s.entries, key)
		delete(s.tickets, key)
		delete(s.retries, key)
	}
}
