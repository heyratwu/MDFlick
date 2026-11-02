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
	refreshCancel         context.CancelFunc
	revision              uint64
}

func (s *documentSession) beginLoad() (context.Context, uint64) {
	if s.refreshCancel != nil {
		s.refreshCancel()
		s.refreshCancel = nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	// The loading/error screens never display the previous parse. Release our
	// ownership before a new worker builds another potentially large document.
	s.prepared = preparedReaderDocument{}
	s.source = ""
	s.generation++
	s.loading, s.failure = true, ""
	return ctx, s.generation
}
func (s *documentSession) accepts(version uint64) bool { return !s.closed && s.generation == version }
func (s *documentSession) close() {
	if s.refreshCancel != nil {
		s.refreshCancel()
	}
	s.closed = true
	s.prepared = preparedReaderDocument{}
	s.source = ""
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
		s.revision++
	}
	return true
}

// Keep the published parse and its image context alive while checking a save.
func (s *documentSession) beginRefresh() (context.Context, uint64) {
	if s.refreshCancel != nil {
		s.refreshCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.refreshCancel = cancel
	s.generation++
	return ctx, s.generation
}

func (s *documentSession) completeRefresh(version uint64, source string, prepared preparedReaderDocument, err error) bool {
	if !s.accepts(version) {
		return false
	}
	if err != nil || source == s.source {
		// A transient read failure during an atomic save must not hide the
		// current preview. A later create/write event retries the read.
		if s.refreshCancel != nil {
			s.refreshCancel()
			s.refreshCancel = nil
		}
		return false
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.refreshCancel = s.refreshCancel, nil
	return s.completePrepared(version, source, prepared, nil)
}
