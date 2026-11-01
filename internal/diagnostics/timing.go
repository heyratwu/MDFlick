package diagnostics

import (
	"context"
	"log"
	"os"
	"time"
)

type Event struct {
	Stage   string
	Elapsed time.Duration
	Bytes   int
}
type observerKey struct{}

func WithObserver(ctx context.Context, observer func(Event)) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}
func Record(ctx context.Context, stage string, started time.Time, bytes int) {
	event := Event{Stage: stage, Elapsed: time.Since(started), Bytes: bytes}
	if observer, ok := ctx.Value(observerKey{}).(func(Event)); ok {
		observer(event)
	}
	if os.Getenv("MDFLICK_TRACE") == "1" {
		log.Printf("pipeline stage=%s elapsed=%s bytes=%d", event.Stage, event.Elapsed, event.Bytes)
	}
}
