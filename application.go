package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
)

type application struct {
	windows   map[string]*readerWindow
	resources *applicationResources
}

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
	return manager.newWindowAt(path, time.Now())
}

func (manager *application) newWindowAt(path string, openedAt time.Time) *readerWindow {
	if manager.resources == nil {
		manager.resources = newApplicationResources()
	}
	a := &readerWindow{documentSession: documentSession{path: path}, resources: manager.resources, openStarted: openedAt}
	a.open = func() { manager.openDialog(a) }
	a.openSibling = func(path string) { manager.switchDocument(a, path) }
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
	a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
		a.win.Update(func() {
			for _, path := range e.Paths {
				if document.IsMarkdownPath(path) {
					manager.openPath(path, a)
				}
			}
		})
	})
	a.openDocument = func(path string) {
		a.win.Update(func() { manager.openPath(path, a) })
	}
	manager.windows[path] = a
	a.win.OnClosed(func() {
		a.close()
		manager.unregisterWindow(a)
		if len(manager.windows) == 0 && manager.resources.diagramPool != nil {
			manager.resources.diagramPool.close()
			manager.resources.diagramPool = nil
		}
	})
	if path != "" {
		a.load()
	}
	return a
}
func (manager *application) openPath(path string, target *readerWindow) {
	openedAt := time.Now()
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
	for _, a := range manager.windows {
		if a.path == absolute {
			a.win.Show()
			a.win.Focus()
			return
		}
	}
	if manager.assignHome(target, absolute) {
		target.openStarted = openedAt
		target.win.SetTitle(filepath.Base(absolute) + " — MDFlick")
		target.load()
		if target.sidebar.open {
			target.updateSidebarDirectory()
		}
		target.win.Invalidate()
		return
	}
	manager.newWindowAt(absolute, openedAt)
}

func (manager *application) unregisterWindow(target *readerWindow) {
	for key, window := range manager.windows {
		if window == target {
			delete(manager.windows, key)
		}
	}
}

// A sidebar selection always replaces this window, even if another window is
// already displaying that path. Keep both windows registered independently.
func (manager *application) replaceWindowPath(target *readerWindow, path string) bool {
	if target == nil || target.closed || !(document.IsMarkdownPath(path) || isPreviewImagePath(path)) {
		return false
	}
	manager.unregisterWindow(target)
	key := path
	if manager.windows[key] != nil {
		key = fmt.Sprintf("%s\x00%p", path, target)
	}
	target.path = path
	manager.windows[key] = target
	return true
}

func (manager *application) switchDocument(target *readerWindow, path string) {
	if target == nil || target.closed || path == target.path {
		return
	}
	if !manager.replaceWindowPath(target, path) {
		return
	}
	if target.watchCancel != nil {
		target.watchCancel()
		target.watchCancel = nil
	}
	target.copiedAt = time.Time{}
	target.openStarted = time.Now()
	target.win.SetTitle(filepath.Base(path) + " — MDFlick")
	target.win.SetRepresentedFile(path)
	target.load()
	target.updateSidebarDirectory()
	target.win.Invalidate()
}
