package main

import "context"

// documentSession owns one document's load lifecycle, independently of MyGo.
// Its state is confined to the UI thread; workers receive only ctx and a version.
type documentSession struct {
	path, source, failure string
	loading, closed       bool
	generation            uint64
	cancel                context.CancelFunc
}

func (s *documentSession) beginLoad() (context.Context, uint64) {
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.generation++
	s.loading, s.failure = true, ""
	return ctx, s.generation
}
func (s *documentSession) accepts(version uint64) bool { return !s.closed && s.generation == version }
func (s *documentSession) close() {
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
}
