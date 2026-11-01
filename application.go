package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"log"
	"path/filepath"
)

type application struct{ windows map[string]*readerWindow }

// assignHome moves an existing welcome window into the document registry.
// A closed or already occupied window must never consume another selection.
func (a *application) assignHome(target *readerWindow, path string) bool {
	if target == nil || target.closed || target.path != "" || a.windows[""] != target {
		return false
	}
	delete(a.windows, "")
	target.path = path
	a.windows[path] = target
	return true
}

func (manager *application) openDialog(target *readerWindow) {
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
			mygo.RunOnMain(func() { manager.openPath(p, target) })
		}
	}()
}
func (manager *application) newWindow(path string) *readerWindow {
	a := &readerWindow{documentSession: documentSession{path: path}}
	a.open = func() { manager.openDialog(a) }
	title := "MDFlick"
	if path != "" {
		title = filepath.Base(path) + " — MDFlick"
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title: title, Width: 900, Height: 720, MinWidth: 480, MinHeight: 360,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: readerTitleBarHeight,
		// Native macOS buttons are 14 points tall: equal 13-point margins
		// center them in the 40-point title bar, including after a resize.
		TrafficLightPosition: &mygo.Point{X: 12, Y: (readerTitleBarHeight - 14) / 2},
		Content:              ui.View(a.view),
	})
	manager.windows[path] = a
	a.win.OnClosed(func() {
		a.documentSession.close()
		a.releaseImages()
		a.resetRenderer()
		delete(manager.windows, a.path)
	})
	if path != "" {
		a.load()
	}
	return a
}
func (manager *application) openPath(path string, target *readerWindow) {
	if target == nil {
		target = manager.windows[""]
	}
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
	manager.newWindow(absolute)
}
