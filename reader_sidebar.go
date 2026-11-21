package main

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
)

//go:embed resources/panel-left.svg
var readerSidebarSVG []byte

//go:embed resources/folder.svg
var readerFolderSVG []byte

//go:embed resources/folder-open.svg
var readerFolderOpenSVG []byte

//go:embed resources/file.svg
var readerFileSVG []byte

//go:embed resources/markdown.svg
var readerMarkdownSVG []byte

//go:embed resources/chevrons-down-up.svg
var readerCollapseFoldersSVG []byte

//go:embed resources/file-image.svg
var readerImageSVG []byte

var (
	readerSidebarIcon         = ui.MustParseSVG(readerSidebarSVG)
	readerFolderIcon          = ui.MustParseSVG(readerFolderSVG)
	readerFolderOpenIcon      = ui.MustParseSVG(readerFolderOpenSVG)
	readerImageIcon           = ui.MustParseSVG(readerImageSVG)
	readerFileIcon            = ui.MustParseSVG(readerFileSVG)
	readerMarkdownIcon        = ui.MustParseSVG(readerMarkdownSVG)
	readerCollapseFoldersIcon = ui.MustParseSVG(readerCollapseFoldersSVG)
)

type sidebarEntry struct {
	name, path string
	enabled    bool
	directory  bool
}
type readerSidebar struct {
	open, loading      bool
	directory, failure string
	entries            []sidebarEntry
	generation         uint64
	list               ui.ListState
	folders            map[string]*sidebarFolder
}

type sidebarFolder struct {
	expanded, loading, loaded bool
	entries                   []sidebarEntry
	failure                   string
}

type sidebarRow struct {
	sidebarEntry
	depth int
}

func isReaderPreviewPath(path string) bool {
	return document.IsMarkdownPath(path) || isPreviewImagePath(path) || isTextPreviewPath(path)
}

func (s *readerSidebar) rows() []sidebarRow {
	var rows []sidebarRow
	var appendEntries func([]sidebarEntry, int)
	appendEntries = func(entries []sidebarEntry, depth int) {
		for _, entry := range entries {
			rows = append(rows, sidebarRow{entry, depth})
			folder := s.folders[entry.path]
			if !entry.directory || folder == nil || !folder.expanded {
				continue
			}
			message := ""
			switch {
			case folder.loading:
				message = "正在读取…"
			case folder.failure != "":
				message = folder.failure
			case folder.loaded && len(folder.entries) == 0:
				message = "空文件夹"
			}
			if message != "" {
				rows = append(rows, sidebarRow{sidebarEntry{name: message, path: entry.path + "\x00status"}, depth + 1})
			} else {
				appendEntries(folder.entries, depth+1)
			}
		}
	}
	appendEntries(s.entries, 0)
	return rows
}

func (a *readerWindow) toggleSidebarFolder(path string) {
	s := &a.sidebar
	if s.folders == nil {
		s.folders = make(map[string]*sidebarFolder)
	}
	folder := s.folders[path]
	if folder == nil {
		folder = &sidebarFolder{}
		s.folders[path] = folder
	}
	folder.expanded = !folder.expanded
	if !folder.expanded || folder.loaded || folder.loading {
		return
	}
	folder.loading, folder.failure = true, ""
	if a.win == nil {
		return
	}
	generation, win := s.generation, a.win
	go func() {
		entries, err := readSidebarEntries(path)
		win.Update(func() { a.completeSidebarFolder(path, generation, entries, err) })
	}()
}

func (a *readerWindow) completeSidebarFolder(path string, generation uint64, entries []sidebarEntry, err error) {
	s := &a.sidebar
	if a.closed || s.generation != generation {
		return
	}
	folder := s.folders[path]
	if folder == nil {
		return
	}
	folder.loading = false
	if err != nil {
		folder.failure = "无法读取文件夹"
		return
	}
	folder.entries, folder.loaded, folder.failure = entries, true, ""
}

func readSidebarEntries(directory string) ([]sidebarEntry, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	entries := make([]sidebarEntry, 0, len(files))
	for _, file := range files {
		path := filepath.Join(directory, file.Name())
		enabled, directory := false, file.IsDir()
		supported := isReaderPreviewPath(path)
		if supported || file.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(path); err == nil {
				directory = info.IsDir()
				enabled = info.Mode().IsRegular() && supported
			}
		}
		entries = append(entries, sidebarEntry{name: file.Name(), path: path, enabled: enabled, directory: directory})
	}
	return entries, nil
}

func (a *readerWindow) toggleSidebar() {
	a.sidebar.open = !a.sidebar.open
	if a.sidebar.open && a.sidebar.directory == "" {
		a.refreshSidebar()
	}
}

// Keep the directory tree and its scroll state when opening one of its descendants.
func (a *readerWindow) updateSidebarDirectory() {
	if a.path != "" && a.sidebar.directory != "" {
		if rel, err := filepath.Rel(a.sidebar.directory, a.path); err == nil && filepath.IsLocal(rel) {
			return
		}
	}
	if a.sidebar.open {
		a.refreshSidebar()
	}
}

func (a *readerWindow) refreshSidebar() {
	s := &a.sidebar
	s.generation++
	s.entries, s.failure, s.loading = nil, "", false
	s.list = ui.ListState{}
	s.folders = make(map[string]*sidebarFolder)
	if a.path == "" {
		s.directory = ""
		return
	}
	s.directory = filepath.Dir(a.path)
	if a.win == nil {
		return
	}
	s.loading = true
	directory, generation, win := s.directory, s.generation, a.win
	go func() {
		entries, err := readSidebarEntries(directory)
		win.Update(func() {
			if a.closed || s.generation != generation {
				return
			}
			s.entries, s.loading = entries, false
			if err != nil {
				s.failure = "无法读取目录：" + err.Error()
			}
		})
	}()
}

func (a *readerWindow) sidebarWidth(c *ui.Context) float32 {
	if !a.sidebar.open {
		return 0
	}
	width, _ := c.Size()
	return min(240, width*.35)
}

func (a *readerWindow) sidebarView(c *ui.Context) {
	s, t := &a.sidebar, c.Theme()
	background := t.Surface
	if !t.Dark {
		background = ui.Hex("#F9F9FA")
	}
	panel := ui.Column(c).Key("directory-sidebar").Width(a.sidebarWidth(c)).Shrink(0).Clip().Background(background)
	hovered := panel.Hovered()
	panel.Children(func() {
		title := "当前目录"
		if s.directory != "" {
			title = filepath.Base(s.directory)
		}
		ui.Row(c).Padding(12, 12, 6, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).Grow(1).FontSize(12).Bold().TextColor(t.TextMuted).MinWidth(0).MaxLines(1).Ellipsis("…")
			button := ui.ButtonBase(c).Key("collapse-sidebar-folders").Size(24, 24).Shrink(0).Radius(4).Center().
				Label("折叠所有文件夹").Tooltip("折叠所有文件夹").IgnoreInput(!hovered)
			if !hovered {
				button.Opacity(0)
			} else if button.Hovered() {
				button.Background(t.TextMuted.Alpha(.08))
			}
			button.Children(func() { ui.Icon(c, readerCollapseFoldersIcon).Size(14, 14).TextColor(t.TextMuted) })
			button.OnClick(func() {
				for _, folder := range s.folders {
					folder.expanded = false
				}
			})
		})
		if a.path == "" {
			ui.Text(c, "打开文件后显示所在目录").FontSize(12).TextColor(t.TextMuted).Padding(12)
			return
		}
		if s.loading {
			ui.Text(c, "正在读取目录…").FontSize(12).Padding(12)
			return
		}
		if s.failure != "" {
			ui.Text(c, s.failure).FontSize(12).Padding(12)
			return
		}
		rows := s.rows()
		ui.List(c.Key("directory-files"), &s.list, len(rows)).Grow(1).Padding(0, 6, 12, 6).
			ItemKey(func(i int) any { return rows[i].path }).Rows(func(row ui.ListRow) {
			c := row.Context
			entry := rows[row.Index]
			enabled := entry.enabled || entry.directory
			expanded := false
			if folder := s.folders[entry.path]; folder != nil {
				expanded = folder.expanded
			}
			button := ui.ButtonBase(c).Height(30).Shrink(0).Margin(2).Padding(0, 8, 0, 8+float32(entry.depth)*16).Gap(7).Radius(5).Justify(ui.Start).AlignItems(ui.Center).Disabled(!enabled).Label(entry.name)
			color := t.Text
			if !enabled {
				color = t.TextMuted.Alpha(.5)
			}
			if entry.path == a.path {
				button.Background(t.TextMuted.Alpha(.15))
			} else if enabled && button.Hovered() {
				button.Background(t.TextMuted.Alpha(.08))
			}
			button.Children(func() {
				icon := readerFileIcon
				if !entry.directory && document.IsMarkdownPath(entry.path) {
					icon = readerMarkdownIcon
				}
				if !entry.directory && isImagePath(entry.path) {
					icon = readerImageIcon
				}
				if entry.directory {
					icon = readerFolderIcon
					if expanded {
						icon = readerFolderOpenIcon
					}
				}
				ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(color)
				ui.Text(c, entry.name).MinWidth(0).FontSize(13).TextColor(color).MaxLines(1).Ellipsis("…").Unselectable()
			})
			button.OnClick(func() {
				if entry.directory {
					a.toggleSidebarFolder(entry.path)
					return
				}
				if entry.enabled && a.openSibling != nil {
					a.openSibling(entry.path)
				}
			})
		})
	})
}
