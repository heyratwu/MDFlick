package main

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/typography"
)

// readerWindow owns native resources. All fields are accessed on the UI thread.
type readerWindow struct {
	onLink    func(string)
	resources *applicationResources
	documentSession
	images      *imageStore
	win         *mygo.Window
	md          *markdown.Options
	diagrams    *diagramService
	blockIndent float32
	scroll      ui.ScrollState
	html        readerHTMLState
	open        func()
	performance readerPerformance
	openStarted time.Time
}

func (a *readerWindow) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
		if a.loading {
			ui.Text(c, "正在读取…").Padding(24)
		} else if a.failure != "" {
			ui.Column(c).Padding(24).Gap(12).Children(func() {
				ui.Text(c, "无法打开文件").FontSize(22).Bold()
				ui.Text(c, a.failure).TextColor(t.TextMuted)
				ui.Button(c, "重试").OnClick(a.load)
			})
		} else if a.path == "" {
			ui.Column(c).Grow(1).Center().Gap(16).Padding(32).Children(func() {
				ui.Text(c, "轻松阅读 Markdown").FontSize(20)
				ui.Text(c, "在 Finder 中选中文件，按空格即可快速预览。").TextColor(t.TextMuted)
				ui.PrimaryButton(c, "打开文件…").OnClick(func() {
					if a.open != nil {
						a.open()
					}
				})
			})
		} else {
			a.observePresentation(c)
			if a.md == nil {
				a.md = &markdown.Options{}
			}
			a.md.Theme = readerTheme(t.Dark)
			a.md.CodeBlock = a.codeBlock
			a.md.BlockStyle = a.readerBlockStyle
			a.md.OnLink = a.linkHandler(c)
			left, prose, _ := readerWidths(c)
			ui.Scroll(c).Key(struct {
				Path       string
				Generation uint64
			}{a.path, a.generation}).TrackScroll(&a.scroll).Grow(1).Background(ui.Hex(typography.Colors(t.Dark).Background)).Children(func() {
				ui.Column(c).Width(prose).Margin(0, 0, 0, left).Padding(40, 0, 120, 0).FontWeight(400).Children(func() {
					markdown.ViewDocument(c, a.prepared.parsed, a.md)
					if a.source == "" {
						ui.Text(c, "此 Markdown 文件为空").TextColor(t.TextMuted)
					}
				})
			})
		}
	})
}

func (a *readerWindow) load() {
	if a.closed {
		return
	}
	if a.resources == nil {
		a.resources = newApplicationResources()
	}
	resources := a.resources
	started := a.openStarted
	if started.IsZero() {
		started = time.Now()
	}
	a.openStarted = time.Time{}
	ctx, generation := a.beginLoad()
	a.scroll = ui.ScrollState{}
	a.performance = readerPerformance{}
	if diagnostics.Enabled() {
		ctx = diagnostics.WithRequest(ctx)
		a.performance = readerPerformance{ctx: ctx, started: started}
	}
	path, win := a.path, a.win
	a.closeDiagrams()
	a.releaseImages()
	go func() {
		source, prepared, err := loadReaderDocument(ctx, path, resources)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() {
			if !a.completePrepared(generation, source, prepared, err) {
				return
			}
			if err != nil {
				return
			}
			diagnostics.Record(ctx, "native.open-to-publish", started, len(source))
			a.html = readerHTMLState{}
			a.resetMarkdown(ctx, path)
			if prepared.hasMermaid && document.NativePolicy().Mermaid {
				if resources.diagramPool == nil {
					resources.diagramPool = newDiagramPool(resources.diagrams)
				}
				backend := newDiagramRenderer(a.win, resources.diagramPool)
				a.diagrams = newDiagramService(backend, func(fn func()) {
					win.Update(func() { fn(); resources.record("native.diagram-result") })
				}, resources.diagramCache)
			}
			a.win.SetRepresentedFile(path)
			_ = mygo.App.AddRecentDocument(path)
		})
	}()
}

func (a *readerWindow) closeDiagrams() {
	a.diagrams.close()
	a.diagrams = nil
}

func (a *readerWindow) close() {
	a.documentSession.close()
	a.releaseImages()
	a.closeDiagrams()
	a.resources.record("native.closed")
	diagnostics.Memory("native.closed")
}

// New options own a new image cache; never copy Options' internal sync.Map.
func (a *readerWindow) resetMarkdown(ctx context.Context, path string) {
	a.releaseImages()
	if a.resources == nil {
		a.resources = newApplicationResources()
	}
	a.images = newImageStore(ctx, path, a.resources.images)
	a.md = &markdown.Options{
		Code:      map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock},
		LoadImage: a.images.load,
	}
}

func (a *readerWindow) releaseImages() {
	if a.images != nil {
		a.images.close()
		a.images = nil
	}
	a.md = nil
}

// Hold a slot through both read and preparation; queued windows do not retain
// file contents. Timeout applies to this operation, not the image-store lifetime.
func loadReaderDocument(parent context.Context, path string, resources *applicationResources) (string, preparedReaderDocument, error) {
	ctx, cancel := context.WithTimeout(parent, document.NativeLimits().Timeout)
	defer cancel()
	if err := acquireWork(ctx, resources.loads); err != nil {
		return "", preparedReaderDocument{}, err
	}
	defer func() { <-resources.loads }()
	started := time.Now()
	source, err := document.LoadContext(ctx, path)
	diagnostics.Record(ctx, "native.read", started, len(source))
	if err != nil {
		return "", preparedReaderDocument{}, err
	}
	prepared, err := prepareReaderDocument(ctx, source)
	if err != nil {
		return "", preparedReaderDocument{}, err
	}
	return source, prepared, nil
}
