package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

const diagramIdleTimeout = 30 * time.Second

type diagramRuntime interface {
	render(context.Context, diagramKey) diagramResult
	alive() bool
	close()
}

type pooledDiagramJob struct {
	start  func() bool
	ctx    context.Context
	key    diagramKey
	create func() (diagramRuntime, error)
	result chan diagramResult
}

// Two lazy workers bound live WebViews, not just Go waits. A worker exclusively
// owns its runtime; cancellation/timeout destroys it before taking another job.
// No hidden top-level window is needed, so last-window quit remains unchanged.
type diagramPool struct {
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	jobs   chan pooledDiagramJob
	slots  chan struct{}
	idle   time.Duration
}

func newDiagramPool(slots chan struct{}) *diagramPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &diagramPool{ctx: ctx, cancel: cancel, jobs: make(chan pooledDiagramJob, 16), slots: slots, idle: diagramIdleTimeout}
}

func (p *diagramPool) close() { p.cancel() }

func (p *diagramPool) render(ctx context.Context, key diagramKey, create func() (diagramRuntime, error)) diagramResult {
	return p.renderWithStart(ctx, key, create, nil)
}

func (p *diagramPool) renderWithStart(ctx context.Context, key diagramKey, create func() (diagramRuntime, error), start func() bool) diagramResult {
	if err := ctx.Err(); err != nil {
		return diagramResult{err: err}
	}
	if err := p.ctx.Err(); err != nil {
		return diagramResult{err: err}
	}
	p.once.Do(func() {
		for range 2 {
			go p.work()
		}
	})
	job := pooledDiagramJob{ctx: ctx, key: key, create: create, result: make(chan diagramResult, 1), start: start}
	select {
	case p.jobs <- job:
	case <-ctx.Done():
		return diagramResult{err: ctx.Err()}
	case <-p.ctx.Done():
		return diagramResult{err: p.ctx.Err()}
	}
	select {
	case result := <-job.result:
		return result
	case <-ctx.Done():
		return diagramResult{err: ctx.Err()}
	case <-p.ctx.Done():
		return diagramResult{err: p.ctx.Err()}
	}
}

func (p *diagramPool) work() {
	var runtime diagramRuntime
	release := func() {
		if runtime != nil {
			runtime.close()
			runtime = nil
		}
	}
	defer release()
	timer := time.NewTimer(p.idle)
	defer timer.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-timer.C:
			release()
		case job := <-p.jobs:
			// Timer callbacks never touch WebKit; all ownership stays on this worker.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			ctx, cancel := context.WithCancel(job.ctx)
			stop := context.AfterFunc(p.ctx, cancel)
			result := diagramResult{err: ctx.Err()}
			usedRuntime := false
			if p.ctx.Err() != nil {
				cancel()
				result.err = p.ctx.Err()
			}
			if result.err == nil && job.start != nil && !job.start() {
				result.err = context.Canceled
			}
			if result.err == nil {
				usedRuntime = true
				if runtime != nil && !runtime.alive() {
					release()
				}
				if runtime == nil {
					runtime, result.err = job.create()
				}
				if result.err == nil {
					if p.slots != nil {
						result.err = acquireWork(ctx, p.slots)
					}
					if result.err == nil {
						result = runtime.render(ctx, job.key)
						if p.slots != nil {
							<-p.slots
						}
						// Another document may have closed the runtime's owner mid-render.
						// Retry on this request's live window, never publish an obsolete image.
						if ctx.Err() == nil && !runtime.alive() {
							release()
							runtime, result.err = job.create()
							if result.err == nil {
								result = runtime.render(ctx, job.key)
							}
						}
					}
				}
			}
			if ctx.Err() != nil {
				result = diagramResult{err: ctx.Err()}
			}
			if usedRuntime && (errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded)) {
				release()
			}
			stop()
			cancel()
			job.result <- result
			timer.Reset(p.idle)
		}
	}
}
