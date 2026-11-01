package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/diagnostics"
)

// Owned by the UI thread. Each load has a separate context/request ID. The
// presentation callback is registered only for ready content and rejects stale
// generations. Timing is surface submission, not physical display scanout.
type readerPerformance struct {
	ctx     context.Context
	started time.Time
	first   bool
}

func (a *readerWindow) observePresentation(c *ui.Context) {
	if a.performance.ctx == nil {
		return
	}
	generation := a.generation
	c.AfterPresent(func() {
		if !a.accepts(generation) || a.loading || a.failure != "" || a.performance.first {
			return
		}
		a.performance.first = true
		diagnostics.Record(a.performance.ctx, "native.open-to-first-present", a.performance.started, len(a.prepared.markdown))
		a.resources.record("native.first-present")
		diagnostics.Memory("native.first-present")
	})
}
