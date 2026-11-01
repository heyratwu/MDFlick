package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/document"
	"mdflick/internal/typography"
)

// readerWindow owns native resources. All fields are accessed on the UI thread.
type readerWindow struct {
	documentSession
	win          *mygo.Window
	md           *markdown.Options
	renderer     *diagramRenderer
	diagrams     map[diagramKey]*diagramResult
	diagramBytes int64
	blockIndent  float32
	open         func()
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
			if a.md == nil {
				a.md = &markdown.Options{}
			}
			a.md.Theme = readerTheme(t.Dark)
			a.md.CodeBlock = a.codeBlock
			a.md.Block = a.wideBlock
			left, prose, _ := readerWidths(c)
			ui.Scroll(c).Key(a.path).Grow(1).Background(ui.Hex(typography.Colors(t.Dark).Background)).Children(func() {
				ui.Column(c).Width(prose).Margin(0, 0, 0, left).Padding(40, 0, 120, 0).FontWeight(400).Children(func() {
					markdown.View(c, a.source, a.md)
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
	ctx, generation := a.beginLoad()
	path, win := a.path, a.win
	a.resetRenderer()
	go func() {
		source, err := document.LoadContext(ctx, path)
		if ctx.Err() != nil {
			return
		}
		hasMermaid := document.DetectMermaid(source)
		if ctx.Err() != nil {
			return
		}
		win.Update(func() {
			if !a.complete(generation, source, err) {
				return
			}
			if err != nil {
				return
			}
			a.resetMarkdown(ctx, path)
			if hasMermaid {
				var renderErr error
				a.renderer, renderErr = newDiagramRenderer(a.win)
				if renderErr != nil {
					log.Println("Mermaid:", renderErr)
				}
			}
			a.win.SetRepresentedFile(path)
			_ = mygo.App.AddRecentDocument(path)
		})
	}()
}

func (a *readerWindow) resetRenderer() {
	if a.renderer != nil {
		a.renderer.close()
		a.renderer = nil
	}
	a.diagrams = make(map[diagramKey]*diagramResult)
	a.diagramBytes = 0
}

// New options own a new image cache; never copy Options' internal sync.Map.
func (a *readerWindow) resetMarkdown(ctx context.Context, path string) {
	a.md = &markdown.Options{
		Code:      map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock},
		LoadImage: documentImages(ctx, filepath.Dir(path)),
	}
}
