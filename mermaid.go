package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
}

// Each window owns one hidden WebView. All diagrams share its offline runtime,
// and the mutex serializes Mermaid's global configuration and DOM operations.
type diagramRenderer struct {
	web    *mygo.WebView
	ready  chan struct{}
	once   sync.Once
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func newDiagramRenderer(win *mygo.Window) (*diagramRenderer, error) {
	web, err := win.NewWebView(mygo.WebViewOptions{URL: "about:blank", Page: mygo.PageOptions{Untrusted: true}})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &diagramRenderer{web: web, ready: make(chan struct{}), ctx: ctx, cancel: cancel}
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

func (r *diagramRenderer) render(key diagramKey) diagramResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	defer cancel()
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
	image, err := ui.DecodeImage(png)
	return diagramResult{image: image, width: result.Width, err: err}
}

func (a *app) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView) {
	if b.Open || a.renderer == nil {
		markdown.DefaultCodeBlock(c, b)
		return
	}
	key := diagramKey{b.Code(), c.Theme().Dark}
	result, exists := a.diagrams[key]
	if !exists {
		// A guarded job is started only once per source and appearance. Publish
		// exclusively through Window.Update; never retain Context or block views.
		a.diagrams[key] = nil
		renderer, generation := a.renderer, a.generation
		go func() {
			rendered := renderer.render(key)
			a.win.Update(func() {
				if a.closed || a.generation != generation || a.renderer != renderer {
					return
				}
				a.diagrams[key] = &rendered
			})
		}()
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
