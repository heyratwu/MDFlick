package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type testDiagramRuntime struct {
	dead     atomic.Bool
	renderFn func(context.Context, diagramKey) diagramResult
	closed   chan struct{}
}

func (r *testDiagramRuntime) alive() bool { return !r.dead.Load() }
func (r *testDiagramRuntime) close() {
	if r.dead.CompareAndSwap(false, true) && r.closed != nil {
		close(r.closed)
	}
}
func (r *testDiagramRuntime) render(ctx context.Context, key diagramKey) diagramResult {
	return r.renderFn(ctx, key)
}

func receiveDiagram[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("diagram worker stalled")
		var zero T
		return zero
	}
}

func TestDiagramPoolBoundsLiveRuntimesAndReuses(t *testing.T) {
	p := newDiagramPool(make(chan struct{}, 2))
	defer p.close()
	var live, peak, created atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	makeRuntime := func() (diagramRuntime, error) {
		n := live.Add(1)
		created.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		r := &countedRuntime{live: &live}
		r.renderFn = func(ctx context.Context, _ diagramKey) diagramResult {
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return diagramResult{err: ctx.Err()}
			}
			return diagramResult{}
		}
		return r, nil
	}
	if created.Load() != 0 {
		t.Fatal("eager runtime creation")
	}
	results := make(chan diagramResult, 4)
	for range 4 {
		go func() { results <- p.render(context.Background(), diagramKey{}, makeRuntime) }()
	}
	receiveDiagram(t, started)
	receiveDiagram(t, started)
	if live.Load() != 2 || peak.Load() > 2 {
		t.Fatal("unbounded WebViews", live.Load(), peak.Load())
	}
	close(release)
	for range 4 {
		if r := receiveDiagram(t, results); r.err != nil {
			t.Fatal(r.err)
		}
	}
	if created.Load() != 2 {
		t.Fatal("runtimes were not reused", created.Load())
	}
}

type countedRuntime struct {
	testDiagramRuntime
	live *atomic.Int32
}

func (r *countedRuntime) close() {
	if !r.dead.Load() {
		r.live.Add(-1)
	}
	r.testDiagramRuntime.close()
}

func TestDiagramPoolCancellationAndIdleRelease(t *testing.T) {
	p := newDiagramPool(nil)
	p.idle = 20 * time.Millisecond
	defer p.close()
	closed := make(chan struct{})
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan diagramResult, 1)
	go func() {
		results <- p.render(ctx, diagramKey{}, func() (diagramRuntime, error) {
			return &testDiagramRuntime{closed: closed, renderFn: func(ctx context.Context, _ diagramKey) diagramResult {
				close(started)
				<-ctx.Done()
				return diagramResult{err: ctx.Err()}
			}}, nil
		})
	}()
	receiveDiagram(t, started)
	cancel()
	if r := receiveDiagram(t, results); r.err != context.Canceled {
		t.Fatal("cancellation lost", r.err)
	}
	receiveDiagram(t, closed)
	idleClosed := make(chan struct{})
	result := p.render(context.Background(), diagramKey{}, func() (diagramRuntime, error) {
		return &testDiagramRuntime{closed: idleClosed, renderFn: func(context.Context, diagramKey) diagramResult { return diagramResult{} }}, nil
	})
	if result.err != nil {
		t.Fatal(result.err)
	}
	receiveDiagram(t, idleClosed)
}

func TestDiagramPoolRecoversWhenOwnerCloses(t *testing.T) {
	p := newDiagramPool(nil)
	defer p.close()
	var created atomic.Int32
	result := p.render(context.Background(), diagramKey{}, func() (diagramRuntime, error) {
		n := created.Add(1)
		r := &testDiagramRuntime{}
		r.renderFn = func(context.Context, diagramKey) diagramResult {
			if n == 1 {
				r.dead.Store(true)
				return diagramResult{err: context.Canceled}
			}
			return diagramResult{width: 42}
		}
		return r, nil
	})
	if result.err != nil || result.width != 42 || created.Load() != 2 {
		t.Fatal("owner closure did not rebuild", result, created.Load())
	}
}

func TestDiagramPoolCloseCancelsActiveAndQueued(t *testing.T) {
	p := newDiagramPool(nil)
	started := make(chan struct{}, 2)
	closed := make(chan struct{}, 2)
	result := make(chan diagramResult, 3)
	create := func() (diagramRuntime, error) {
		r := &testDiagramRuntime{closed: make(chan struct{})}
		r.renderFn = func(ctx context.Context, _ diagramKey) diagramResult {
			started <- struct{}{}
			<-ctx.Done()
			return diagramResult{err: ctx.Err()}
		}
		go func() { <-r.closed; closed <- struct{}{} }()
		return r, nil
	}
	for range 2 {
		go func() { result <- p.render(context.Background(), diagramKey{}, create) }()
	}
	receiveDiagram(t, started)
	receiveDiagram(t, started)
	go func() { result <- p.render(context.Background(), diagramKey{}, create) }()
	p.close()
	for range 3 {
		if r := receiveDiagram(t, result); r.err != context.Canceled {
			t.Fatal(r.err)
		}
	}
	for range 2 {
		receiveDiagram(t, closed)
	}
}
