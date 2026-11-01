package diagnostics

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

type Event struct {
	Stage   string
	Elapsed time.Duration
	Bytes   int
}
type observerKey struct{}
type requestKey struct{}

var nextRequest atomic.Uint64

// WithRequest correlates all stages of one open/reload without logging paths.
func WithRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestKey{}, nextRequest.Add(1))
}

func WithObserver(ctx context.Context, observer func(Event)) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}
func Record(ctx context.Context, stage string, started time.Time, bytes int) {
	event := Event{Stage: stage, Elapsed: time.Since(started), Bytes: bytes}
	if observer, ok := ctx.Value(observerKey{}).(func(Event)); ok {
		observer(event)
	}
	if Enabled() {
		id, _ := ctx.Value(requestKey{}).(uint64)
		log.Printf("pipeline request=%d stage=%s elapsed=%s bytes=%d", id, event.Stage, event.Elapsed, event.Bytes)
	}
}
