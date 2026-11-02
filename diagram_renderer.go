package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"sort"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mdflick/internal/mermaid"
)

type diagramKey struct {
	code string
	dark bool
}
type diagramResult struct {
	image  ui.ImageSource
	width  float32
	height float32
	err    error
	bytes  int64
}

// A document owns only its cancellation context and bounded submission queue.
// Native runtimes are borrowed from the application pool when work executes.
type diagramRenderer struct {
	mu      sync.Mutex
	pending map[diagramKey]*diagramWork
	jobs    chan diagramJob
	ctx     context.Context
	cancel  context.CancelFunc
	pool    *diagramPool
	create  func() (diagramRuntime, error)
}

func newDiagramRenderer(win *mygo.Window, pool *diagramPool) *diagramRenderer {
	ctx, cancel := context.WithCancel(context.Background())
	// At most 128 keys are resident in a document service. Queueing them here
	// avoids making a full application queue a permanently cached render error.
	r := &diagramRenderer{ctx: ctx, cancel: cancel, jobs: make(chan diagramJob, 128), pool: pool,
		create: func() (diagramRuntime, error) { return newDiagramWebRuntime(win) }}
	go r.work()
	return r
}

func (r *diagramRenderer) close() { r.cancel() }

// A runtime stays attached to its original native window. It may evaluate jobs
// for other documents without moving its view; owner closure invalidates it.
type diagramWebRuntime struct {
	web    *mygo.WebView
	ready  chan struct{}
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
	off    func()
}

func newDiagramWebRuntime(win *mygo.Window) (*diagramWebRuntime, error) {
	web, err := win.NewWebView(mygo.WebViewOptions{URL: "about:blank", Page: mygo.PageOptions{Untrusted: true}})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &diagramWebRuntime{web: web, ready: make(chan struct{}), ctx: ctx, cancel: cancel}
	r.off = win.OnClosed(cancel)
	web.Page().OnDidFinishLoad(func() {
		go func() {
			ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
			defer cancel()
			ready, err := web.Page().EvalContext(ctx, "typeof window.mdflickPNG === 'function'")
			if err == nil && ready == true {
				r.once.Do(func() { close(r.ready) })
			}
		}()
	})
	web.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if e.URL != "about:blank" {
			e.PreventDefault()
		}
	})
	web.Page().SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
	web.Page().LoadHTML(mermaid.HTML(), "")
	return r, nil
}

func (r *diagramWebRuntime) close()      { r.cancel(); r.off(); r.web.Destroy() }
func (r *diagramWebRuntime) alive() bool { return !r.web.IsDestroyed() }

func (r *diagramWebRuntime) render(ctx context.Context, key diagramKey) diagramResult {
	if err := ctx.Err(); err != nil {
		return diagramResult{err: err}
	}
	select {
	case <-r.ready:
	case <-r.ctx.Done():
		return diagramResult{err: r.ctx.Err()}
	case <-ctx.Done():
		return diagramResult{err: ctx.Err()}
	}
	args, _ := json.Marshal([]any{key.code, key.dark})
	value, err := r.web.Page().EvalContext(ctx, "window.mdflickPNG(..."+string(args)+")")
	if err != nil {
		return diagramResult{err: err}
	}
	data, _ := json.Marshal(value)
	var result struct {
		PNG   string  `json:"png"`
		Width float32 `json:"width"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return diagramResult{err: err}
	}
	png, err := base64.StdEncoding.DecodeString(result.PNG)
	if err != nil {
		return diagramResult{err: err}
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		return diagramResult{err: err}
	}
	pixels := int64(config.Width) * int64(config.Height)
	if pixels <= 0 || pixels > maxImagePixels {
		return diagramResult{err: errors.New("图表尺寸过大")}
	}
	bitmap, err := ui.DecodeImage(png)
	return diagramResult{image: bitmap, width: result.Width, height: float32(config.Height) * result.Width / float32(config.Width), err: err, bytes: pixels * 4}
}

type diagramWork struct {
	cancel  context.CancelFunc
	running bool
}

type diagramJob struct {
	work   *diagramWork
	ctx    context.Context
	cancel context.CancelFunc
	key    diagramKey
	done   func(diagramResult)
}

// Deadlines include queue time. One worker owns Mermaid's global DOM state.
func (r *diagramRenderer) submit(key diagramKey, done func(diagramResult)) bool {
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = make(map[diagramKey]*diagramWork)
	}
	work := &diagramWork{cancel: cancel}
	job := diagramJob{ctx: ctx, cancel: cancel, key: key, done: done, work: work}
	if r.ctx.Err() != nil {
		cancel()
		return false
	}
	select {
	case r.jobs <- job:
		r.pending[key] = work
		return true
	default:
		cancel()
		return false
	}
}
func (r *diagramRenderer) work() {
	r.run(func(ctx context.Context, key diagramKey, start func() bool) diagramResult {
		return r.pool.renderWithStart(ctx, key, r.create, start)
	})
}

func (r *diagramRenderer) run(render func(context.Context, diagramKey, func() bool) diagramResult) {
	for {
		select {
		case <-r.ctx.Done():
			for {
				select {
				case job := <-r.jobs:
					job.cancel()
				default:
					return
				}
			}
		case job := <-r.jobs:
			result := diagramResult{err: job.ctx.Err()}
			if result.err == nil {
				start := func() bool {
					r.mu.Lock()
					defer r.mu.Unlock()
					if job.ctx.Err() != nil {
						return false
					}
					if job.work != nil {
						job.work.running = true
					}
					return true
				}
				if r.pool == nil && !start() {
					result.err = job.ctx.Err()
				} else {
					result = render(job.ctx, job.key, start)
				}
			}
			job.cancel()
			r.mu.Lock()
			if r.pending[job.key] == job.work {
				delete(r.pending, job.key)
			}
			r.mu.Unlock()
			if r.ctx.Err() == nil {
				job.done(result)
			}
		}
	}
}

// Only queued work is cancelled. A render already using WebKit finishes so
// ordinary scrolling never tears down a healthy runtime.
func (r *diagramRenderer) reconcile(wanted map[diagramKey]float32) map[diagramKey]bool {
	running := make(map[diagramKey]bool)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, work := range r.pending {
		if work.running {
			running[key] = true
		}
		if _, keep := wanted[key]; !keep && !work.running {
			work.cancel()
		}
	}
	var jobs []diagramJob
	for {
		select {
		case job := <-r.jobs:
			if job.ctx.Err() == nil {
				jobs = append(jobs, job)
			} else if r.pending[job.key] == job.work {
				delete(r.pending, job.key)
			}
		default:
			goto drained
		}
	}
drained:
	sort.SliceStable(jobs, func(i, j int) bool { return wanted[jobs[i].key] < wanted[jobs[j].key] })
	for _, job := range jobs {
		r.jobs <- job
	}
	return running
}
