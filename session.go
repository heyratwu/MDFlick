package main

import "context"

// documentSession owns one document's load lifecycle, independently of MyGo.
// Its state is confined to the UI thread; workers receive only ctx and a version.
type documentSession struct {
	prepared              preparedReaderDocument
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

// complete is the publication boundary for asynchronous reads. A stale success
// or failure must not alter the current session, including its loading state.
func (s *documentSession) complete(version uint64, source string, err error) bool {
	if !s.accepts(version) {
		return false
	}
	s.loading = false
	if err != nil {
		s.failure = err.Error()
	} else {
		s.failure = ""
		s.source = source
	}
	return true
}

// Publish raw and derived content atomically at the session boundary.
func (s *documentSession) completePrepared(version uint64, source string, prepared preparedReaderDocument, err error) bool {
	if !s.complete(version, source, err) {
		return false
	}
	if err == nil {
		s.prepared = prepared
	}
	return true
}
