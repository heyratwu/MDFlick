package main

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/document"
)

type app struct {
	windows               map[string]*app
	win                   *mygo.Window
	path, source, failure string
	loading, closed       bool
	generation            uint64
	md                    markdown.Options
	renderer              *diagramRenderer
	diagrams              map[diagramKey]*diagramResult
	open                  func()
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
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
			a.md.Theme = &markdown.Theme{FontSize: 16, LineHeight: 1.7}
			ui.Scroll(c).Key(a.path).Grow(1).Padding(28).Children(func() {
				markdown.View(c, a.source, &a.md)
				if a.source == "" {
					ui.Text(c, "此 Markdown 文件为空").TextColor(t.TextMuted)
				}
			})
		}
	})
}

func (a *app) load() {
	a.generation++
	generation, path := a.generation, a.path
	a.loading, a.failure = true, ""
	go func() {
		source, err := document.Load(path)
		a.win.Update(func() {
			if a.closed || generation != a.generation {
				return
			}
			a.loading = false
			if err != nil {
				a.failure = err.Error()
				return
			}
			a.source = source
			if a.renderer != nil {
				a.renderer.close()
				a.renderer = nil
			}
			a.diagrams = make(map[diagramKey]*diagramResult)
			a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": a.mermaidBlock}
			if strings.Contains(source, "mermaid") {
				var renderErr error
				a.renderer, renderErr = newDiagramRenderer(a.win)
				if renderErr != nil {
					log.Println("Mermaid:", renderErr)
				}
			}
			a.md.LoadImage = markdown.FileImages(filepath.Dir(path))
			a.win.SetRepresentedFile(path)
			_ = mygo.App.AddRecentDocument(path)
		})
	}()
}

// assignHome moves an existing welcome window into the document registry.
// A closed or already occupied window must never consume another selection.
func (a *app) assignHome(target *app, path string) bool {
	if target == nil || target.closed || target.path != "" || a.windows[""] != target {
		return false
	}
	delete(a.windows, "")
	target.path = path
	a.windows[path] = target
	return true
}

func main() {
	manager := &app{windows: make(map[string]*app)}
	var openPath func(string, *app)
	openDialog := func(target *app) {
		var parent *mygo.Window
		if target != nil {
			parent = target.win
		}
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "打开 Markdown", Multiple: true,
				Filters: []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md", "markdown"}}}})
			if err != nil {
				log.Println(err)
				return
			}
			for _, path := range paths {
				p := path
				mygo.RunOnMain(func() { openPath(p, target) })
			}
		}()
	}
	newWindow := func(path string) *app {
		a := &app{path: path}
		a.open = func() { openDialog(a) }
		title := "MDFlick"
		if path != "" {
			title = filepath.Base(path) + " — MDFlick"
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: title, Width: 900, Height: 720, MinWidth: 480, MinHeight: 360, Content: ui.View(a.view)})
		manager.windows[path] = a
		a.win.OnClosed(func() {
			a.closed = true
			if a.renderer != nil {
				a.renderer.close()
			}
			delete(manager.windows, a.path)
		})
		if path != "" {
			a.load()
		}
		return a
	}
	openPath = func(path string, target *app) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			log.Println(err)
			return
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		if a := manager.windows[absolute]; a != nil {
			a.win.Show()
			a.win.Focus()
			return
		}
		if manager.assignHome(target, absolute) {
			target.win.SetTitle(filepath.Base(absolute) + " — MDFlick")
			target.load()
			target.win.Invalidate()
			return
		}
		newWindow(absolute)
	}
	mygo.App.SetName("MDFlick")
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				var target *app
				for _, a := range manager.windows {
					if a.win == w {
						target = a
						break
					}
				}
				openDialog(target)
			}},
			{Role: mygo.RoleOpenRecent},
			{Label: "重新载入", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				for _, a := range manager.windows {
					if a.win == w && a.path != "" {
						a.load()
					}
				}
			}},
			mygo.Separator(), {Role: mygo.RoleClose},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{{Role: mygo.RoleCopy}, {Role: mygo.RoleSelectAll}}},
		{Role: mygo.RoleWindowMenu},
	}))
	mygo.App.OnOpenFile(func(path string) { mygo.App.WhenReady(func() { openPath(path, nil) }) })
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			newWindow("")
		}
	})
	mygo.App.WhenReady(func() {
		if len(manager.windows) == 0 {
			newWindow("")
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
