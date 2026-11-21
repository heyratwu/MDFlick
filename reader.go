package main

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
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
	textPreview      *editor.State
	preview          ui.ImageSource
	images           *imageStore
	imageScheduler   *imageScheduler
	win              *mygo.Window
	md               *markdown.Options
	diagrams         *diagramService
	diagramFrame     map[diagramKey]float32
	blockIndent      float32
	scroll           ui.ScrollState
	find             readerFind
	html             readerHTMLState
	open             func()
	openDocument     func(string)
	openSibling      func(string)
	sidebar          readerSidebar
	performance      readerPerformance
	openStarted      time.Time
	copiedAt         time.Time
	watchCancel      context.CancelFunc
	switchCancel     context.CancelFunc
	switchGeneration uint64
}

// A sidebar replacement is prepared while the published preview stays alive.
type readerLoadResult struct {
	source   string
	prepared preparedReaderDocument
	text     *editor.State
	preview  ui.ImageSource
	images   *imageStore
	err      error
}

func (a *readerWindow) view(c *ui.Context) {
	t := *c.Theme()
	t.Scrollbar.A -= t.Scrollbar.A / 4
	c.SetTheme(&t)
	if a.prepared.parsed != nil || a.textPreview != nil {
		c.OnShortcut(ui.Cmd, ui.KeyF, a.showFind)
		if a.prepared.parsed != nil && a.find.open {
			c.OnShortcut(0, ui.KeyEscape, a.find.hide)
		}
	}
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
		ui.Row(c).Key("reader-panes").Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.sidebar.open {
				a.sidebarView(c)
			}
			ui.Column(c).Key("reader-document").Grow(1).MinWidth(0).Clip().Children(func() {
				a.documentView(c)
			})
		})
	})
}

func (a *readerWindow) documentView(c *ui.Context) {
	t := c.Theme()
	if a.loading {
		ui.Text(c, "正在读取…").Padding(24)
	} else if a.failure != "" {
		ui.Column(c).Padding(24).Gap(12).Children(func() {
			ui.Text(c, "无法打开文件").FontSize(22).Bold()
			ui.Text(c, a.failure).TextColor(t.TextMuted)
			ui.Button(c, "重试").OnClick(a.load)
		})
	} else if a.path == "" {
		// The extra 100 DIPs below move the centered welcome content up 50.
		ui.Column(c).Grow(1).Center().Gap(16).Padding(32, 32, 132, 32).Children(func() {
			ui.Text(c, "轻松阅读 Markdown").FontSize(20)
			ui.Text(c, "在 Finder 中选中文件，按空格即可快速预览。").TextColor(t.TextMuted)
			// Match a regular button's 1-DIP border without changing the accent face.
			ui.PrimaryButton(c, "打开文件…").Border(1, ui.Transparent).OnClick(func() {
				if a.open != nil {
					a.open()
				}
			})
		})
	} else if isPreviewImagePath(a.path) {
		a.imagePreview(c)
	} else if a.textPreview != nil {
		a.textPreviewView(c)
	} else {
		a.observePresentation(c)
		if a.md == nil {
			a.md = &markdown.Options{}
		}
		a.md.Theme = readerTheme(t.Dark)
		a.md.CodeBlock = a.codeBlock
		a.md.HighlightLimit = readerHighlightLimit
		a.md.BlockStyle = a.readerBlockStyle
		a.md.Block = a.nativeHTMLBlock
		a.md.OnLink = a.linkHandler(c)
		a.md.LinkIcon = func(dest string, size float32) (markdown.InlineObject, bool) {
			return localDocumentIcon(dest, size, a.md.Theme.Link)
		}
		finishDiagrams := a.beginDiagramFrame(c)
		defer finishDiagrams()
		finishImages := a.beginImageFrame(c)
		defer finishImages()
		left, prose, _ := a.readerWidths(c)
		barTop := float32(8)
		if c.TitleBar().Height != 0 {
			barTop += readerTitleBarHeight
		}
		scroll := ui.Scroll(c).Key(struct {
			Path     string
			Revision uint64
		}{a.path, a.revision}).TrackScroll(&a.scroll).Grow(1).Background(ui.Hex(typography.Colors(t.Dark).Background))
		if a.find.open {
			scroll.SearchScrollbar(ui.Hex("#DDA600"), ui.Hex("#F58119"))
		}
		scroll.Children(func() {
			top := float32(40)
			if a.find.open {
				top = readerFindInset
			}
			content := ui.Column(c).Width(prose).Margin(0, 0, 0, left).Padding(top, 0, 120, 0).FontWeight(400).Children(func() {
				if properties := a.prepared.frontMatter; properties != nil {
					readerProperties(c, properties, a.path, prose)
				}
				c.Key("document-body")
				markdown.ViewDocument(c, a.prepared.parsed, a.md)
				if a.source == "" {
					ui.Text(c, "此 Markdown 文件为空").TextColor(t.TextMuted).Unselectable()
				}
			})
			if a.find.open {
				a.find.highlight(content, a.revision, t.Dark)
			}
		})
		if a.find.open {
			a.find.bar(c, barTop)
		}
	}
}

func (a *readerWindow) load() {
	a.loadDocument(false)
}

func (a *readerWindow) refresh() {
	a.loadDocument((a.prepared.parsed != nil || a.textPreview != nil) && !a.loading && a.failure == "")
}

func (a *readerWindow) loadDocument(refresh bool) {
	a.loadDocumentResult(refresh, nil)
}

func (a *readerWindow) loadDocumentResult(refresh bool, ready *readerLoadResult) {
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
	var ctx context.Context
	var generation uint64
	previous := a.source
	if refresh {
		ctx, generation = a.beginRefresh()
	} else {
		ctx, generation = a.beginLoad()
		a.textPreview = nil
		a.scroll = ui.ScrollState{}
		a.find.reset()
		a.find.focus = a.find.open
	}
	a.performance = readerPerformance{}
	if diagnostics.Enabled() {
		ctx = diagnostics.WithRequest(ctx)
		a.performance = readerPerformance{ctx: ctx, started: started}
	}
	path, win := a.path, a.win
	// Establish the watch before the first read so a save during opening is
	// not missed. Its lifetime belongs to the window, not to any single load.
	var watchCtx context.Context
	if a.watchCancel == nil && path != "" {
		watchCtx, a.watchCancel = context.WithCancel(context.Background())
	}
	if !refresh {
		a.closeDiagrams()
		a.releaseImages()
	}
	var previewStore *imageStore
	if isPreviewImagePath(path) && ready == nil {
		a.images = newImageStore(ctx, path, resources.images)
		previewStore = a.images
	}
	if watchCtx != nil {
		go func() {
			watchReaderFile(watchCtx, path, func() {
				win.Update(func() {
					if !a.closed && a.path == path {
						a.refresh()
					}
				})
			})
		}()
	}
	publish := func(result readerLoadResult) {
		source, prepared, err := result.source, result.prepared, result.err
		if isPreviewImagePath(path) {
			if err != nil && result.images != nil {
				result.images.Close()
			}
			if a.complete(generation, "", err) && err == nil {
				a.preview = result.preview
				if result.images != nil {
					a.images = result.images
				}
				a.win.SetRepresentedFile(path)
			}
			return
		}
		if refresh {
			if !a.completeRefresh(generation, source, prepared, err) {
				return
			}
			a.closeDiagrams()
		} else {
			if !a.completePrepared(generation, source, prepared, err) {
				return
			}
		}
		if err != nil {
			return
		}
		diagnostics.Record(ctx, "native.open-to-publish", started, len(source))
		a.textPreview = result.text
		a.win.SetRepresentedFile(path)
		_ = mygo.App.AddRecentDocument(path)
		if result.text != nil {
			return
		}
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
	}
	if ready != nil {
		publish(*ready)
		return
	}
	go func() {
		if previewStore != nil {
			image, err := previewStore.Load(path)
			win.Update(func() {
				publish(readerLoadResult{preview: image, err: err})
			})
			return
		}
		result := loadReaderFile(ctx, path, resources, previous, refresh)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() {
			publish(result)
		})
	}()
}

func (a *readerWindow) closeDiagrams() {
	a.diagrams.close()
	a.diagrams = nil
	a.diagramFrame = nil
}

func (a *readerWindow) close() {
	a.cancelSidebarSwitch()
	a.sidebar.generation++
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
	a.documentSession.close()
	a.textPreview = nil
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
	var renderSVG svgImageRenderer
	if a.win != nil {
		if a.resources.diagramPool == nil {
			a.resources.diagramPool = newDiagramPool(a.resources.diagrams)
		}
		renderSVG = newSVGImageRenderer(a.win, a.resources.diagramPool)
	}
	a.images = newImageStore(ctx, path, a.resources.images, renderSVG)
	a.imageScheduler = newImageScheduler(a.images)
	a.md = &markdown.Options{
		Code:      map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock},
		LoadImage: a.images.Load,
	}
}

func (a *readerWindow) releaseImages() {
	a.preview = nil
	a.imageScheduler = nil
	if a.images != nil {
		a.images.Close()
		a.images = nil
	}
	a.md = nil
}

// Hold a slot through both read and preparation; queued windows do not retain
// file contents. Timeout applies to this operation, not the image-store lifetime.
func loadReaderFile(parent context.Context, path string, resources *applicationResources, previous string, compare bool) readerLoadResult {
	ctx, cancel := context.WithTimeout(parent, document.NativeLimits().Timeout)
	defer cancel()
	if err := acquireWork(ctx, resources.loads); err != nil {
		return readerLoadResult{err: err}
	}
	defer func() { <-resources.loads }()
	started := time.Now()
	text := isTextPreviewPath(path)
	var source string
	var err error
	if text {
		source, err = readTextPreview(ctx, path)
	} else {
		source, err = document.LoadContext(ctx, path)
	}
	diagnostics.Record(ctx, "native.read", started, len(source))
	if err != nil {
		return readerLoadResult{err: err}
	}
	if compare && source == previous {
		return readerLoadResult{source: source}
	}
	if text {
		return readerLoadResult{source: source, text: newTextPreview(path, source)}
	}
	prepared, err := prepareReaderDocument(ctx, source)
	if err != nil {
		return readerLoadResult{err: err}
	}
	return readerLoadResult{source: source, prepared: prepared}
}
