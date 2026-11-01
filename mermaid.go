package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/mermaid"
)

type diagramKey struct {
	code string
	dark bool
}
type diagramResult struct {
	image ui.ImageSource
	width float32
	err   error
	bytes int64
}

// Each window owns one hidden WebView. All diagrams share its offline runtime,
// and one worker serializes Mermaid's global configuration and DOM operations.
type diagramRenderer struct {
	web    *mygo.WebView
	ready  chan struct{}
	once   sync.Once
	jobs   chan diagramJob
	ctx    context.Context
	cancel context.CancelFunc
}

func newDiagramRenderer(win *mygo.Window) (*diagramRenderer, error) {
	web, err := win.NewWebView(mygo.WebViewOptions{URL: "about:blank", Page: mygo.PageOptions{Untrusted: true}})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &diagramRenderer{web: web, ready: make(chan struct{}), ctx: ctx, cancel: cancel, jobs: make(chan diagramJob, 16)}
	go r.work()
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

func (r *diagramRenderer) close() { r.cancel(); r.web.Destroy() }

func (r *diagramRenderer) render(ctx context.Context, key diagramKey) diagramResult {
	if err := ctx.Err(); err != nil {
		return diagramResult{err: err}
	}
	select {
	case <-r.ready:
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
	return diagramResult{image: bitmap, width: result.Width, err: err, bytes: pixels * 4}
}

func (a *readerWindow) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView) {
	if b.Open || a.renderer == nil {
		markdown.DefaultCodeBlock(c, b)
		return
	}
	key := diagramKey{b.Code(), c.Theme().Dark}
	result, exists := a.diagrams[key]
	if !exists {
		// Admission limits avoid both unbounded metadata and cache eviction/re-render
		// loops when every Markdown block is constructed again on each frame.
		if len(a.diagrams) >= 128 {
			markdown.DefaultCodeBlock(c, b)
			return
		}
		renderer, generation, win := a.renderer, a.generation, a.win
		a.diagrams[key] = nil
		accepted := renderer.submit(key, func(rendered diagramResult) {
			win.Update(func() {
				a.commitDiagram(generation, renderer, key, rendered)
			})
		})
		if !accepted {
			a.diagrams[key] = &diagramResult{err: errors.New("图表渲染队列已满，重新载入可重试")}
		}
		result = a.diagrams[key]
	}

	if result == nil {
		ui.Text(c, "正在渲染 Mermaid 图表…").TextColor(c.Theme().TextMuted)
		markdown.DefaultCodeBlock(c, b)
		return
	}
	if result.err != nil {
		ui.Text(c, "Mermaid 图表无法渲染，显示源码").TextColor(c.Theme().TextMuted).Tooltip(fmt.Sprint(result.err))
		markdown.DefaultCodeBlock(c, b)
		return
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Mermaid").TextColor(c.Theme().TextMuted)
		ui.Spacer(c)
		b.CopyButton(c)
	})
	ui.Image(c, result.image).Width(result.width).MaxWidthPercent(100).Label("Mermaid 图表").Role(ui.RoleImage)
}

const diagramCacheBudget int64 = 64 << 20

type diagramJob struct {
	ctx    context.Context
	cancel context.CancelFunc
	key    diagramKey
	done   func(diagramResult)
}

// Deadlines include queue time. One worker owns Mermaid's global DOM state.
func (r *diagramRenderer) submit(key diagramKey, done func(diagramResult)) bool {
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	job := diagramJob{ctx, cancel, key, done}
	if r.ctx.Err() != nil {
		cancel()
		return false
	}
	select {
	case r.jobs <- job:
		return true
	default:
		cancel()
		return false
	}
}
func (r *diagramRenderer) work() { r.run(r.render) }

func (r *diagramRenderer) run(render func(context.Context, diagramKey) diagramResult) {
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
				result = render(job.ctx, job.key)
			}
			job.cancel()
			if r.ctx.Err() == nil {
				job.done(result)
			}
		}
	}
}
func (a *readerWindow) storeDiagram(key diagramKey, result diagramResult) {
	if result.bytes > diagramCacheBudget-a.diagramBytes {
		result = diagramResult{err: errors.New("图表缓存已达内存上限，显示源码")}
	}
	a.diagramBytes += result.bytes
	a.diagrams[key] = &result
}

func (a *readerWindow) commitDiagram(generation uint64, renderer *diagramRenderer, key diagramKey, result diagramResult) {
	if !a.accepts(generation) || a.renderer != renderer {
		return
	}
	a.storeDiagram(key, result)
}
