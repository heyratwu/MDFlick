package main

import (
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const readerFindInset = 62

type readerFind struct {
	open, focus, reveal bool
	query               string
	current, count      int
	revision            uint64
	cache               map[uint64]readerFindText
}

type readerFindText struct {
	text   string
	starts []uint32
}

func (a *readerWindow) showFind() {
	if a.closed || a.loading || a.failure != "" {
		return
	}
	if a.textPreview != nil {
		a.textPreview.Run(editor.Find)
		return
	}
	if a.prepared.parsed == nil {
		return
	}
	if !a.find.open {
		a.find.reveal = true
	}
	a.find.open, a.find.focus = true, true
}

func (s *readerFind) hide() {
	s.open, s.focus = false, false
	s.reset()
}

func (s *readerFind) reset() {
	s.current, s.count = 0, 0
	s.cache = nil
	s.reveal = true
}

func (s *readerFind) move(delta int) {
	if s.count > 0 {
		s.current = (s.current + delta + s.count) % s.count
		s.reveal = true
	}
}

func (s *readerFind) highlight(content ui.Element, revision uint64, dark bool) {
	if s.revision != revision {
		s.revision = revision
		s.reset()
	}
	if s.query == "" {
		return
	}
	if s.cache == nil {
		s.cache = make(map[uint64]readerFindText)
	}
	query := strings.ToLower(s.query)
	length := utf8.RuneCountInString(query)
	s.count = 0
	content.VisitDocumentText(func(element ui.Element, text string) {
		id := element.ID()
		found := s.cache[id]
		if found.text != text {
			found = readerFindText{text: text, starts: readerMatchStarts(text, query, length)}
			s.cache[id] = found
		}
		s.count += len(found.starts)
	})
	if s.current >= s.count {
		s.current = max(0, s.count-1)
		s.reveal = true
	}
	if s.count == 0 {
		return
	}
	normal, active := ui.Hex("#FFEB65"), ui.Hex("#FFAF37")
	if dark {
		normal, active = ui.Hex("#6F5900"), ui.Hex("#A44C00")
	}
	current := s.current
	content.VisitDocumentText(func(element ui.Element, _ string) {
		starts := s.cache[element.ID()].starts
		if len(starts) == 0 {
			return
		}
		index := -1
		if current >= 0 && current < len(starts) {
			index = current
		}
		element.HighlightMatches(starts, length, index, normal, active, readerFindInset)
		if index >= 0 && s.reveal {
			element.ScrollIntoView()
			s.reveal = false
		}
		current -= len(starts)
	})
}

func readerMatchStarts(text, foldedQuery string, length int) []uint32 {
	// ToLower maps one rune to one rune, even if its UTF-8 width changes.
	// Count runes in the folded text so the offsets still fit the original.
	var starts []uint32
	offset := 0
	for rest := strings.ToLower(text); ; {
		i := strings.Index(rest, foldedQuery)
		if i < 0 {
			return starts
		}
		offset += utf8.RuneCountInString(rest[:i])
		starts = append(starts, uint32(offset))
		offset += length
		rest = rest[i+len(foldedQuery):]
	}
}

func (s *readerFind) bar(c *ui.Context, top float32) {
	t := c.Theme()
	if runtime.GOOS == "darwin" {
		c.OnShortcut(ui.Cmd, ui.KeyG, func() { s.move(1) })
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyG, func() { s.move(-1) })
	} else {
		c.OnShortcut(0, ui.KeyF3, func() { s.move(1) })
		c.OnShortcut(ui.Shift, ui.KeyF3, func() { s.move(-1) })
	}
	ui.Row(c).Key("document-find").Absolute().Top(top).Right(20).
		Width(384).Height(48).Padding(0, 10).Gap(6).AlignItems(ui.Center).
		Role(ui.RoleToolbar).Label("在文档中搜索").Background(t.Surface).
		Border(1, t.Border).Radius(8).Shadow(0, 4, 10, 0, ui.RGBA(0, 0, 0, .24)).Children(func() {
		input := ui.TextInputBase(c, &s.query).Grow(1).MinWidth(0).Height(32).Padding(0, 5).
			Label("在文档中搜索").Placeholder("在文档中搜索").OnChange(s.reset).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter {
					return false
				}
				switch k.Mods {
				case 0:
					s.move(1)
				case ui.Shift:
					s.move(-1)
				default:
					return false
				}
				return true
			})
		if s.focus {
			input.Focus().SetTextSelection(0, utf8.RuneCountInString(s.query))
			s.focus = false
		}
		status := ""
		if s.query != "" {
			status = "0/0"
			if s.count > 0 {
				status = strconv.Itoa(s.current+1) + "/" + strconv.Itoa(s.count)
			}
		}
		ui.Text(c, status).Role(ui.RoleStatus).FontSize(12).FontFeatures("tnum").TextColor(t.TextMuted)
		ui.Box(c).Size(1, 26).Background(t.Border).Shrink(0)
		readerFindButton(c, "上一个匹配项", -1, s.count == 0, func() { s.move(-1) })
		readerFindButton(c, "下一个匹配项", 1, s.count == 0, func() { s.move(1) })
		readerFindButton(c, "关闭搜索", 0, false, s.hide)
	})
}

func readerFindButton(c *ui.Context, label string, direction float32, disabled bool, action func()) {
	button := ui.ButtonBase(c).Size(32, 32).Shrink(0).Radius(5).Label(label).Tooltip(label).
		Disabled(disabled).OnClick(action)
	t := c.Theme()
	color := t.TextMuted
	if disabled {
		color.A /= 2
	} else if button.Hovered() || button.Focused() {
		button.Background(t.SurfaceHover)
	}
	button.Draw(func(p *ui.Painter, r ui.Rect) {
		x, y := r.X+r.W/2, r.Y+r.H/2
		var path ui.Path
		if direction == 0 {
			path.MoveTo(x-3, y-3).LineTo(x+3, y+3)
			path.MoveTo(x+3, y-3).LineTo(x-3, y+3)
		} else {
			path.MoveTo(x-4, y-2*direction).LineTo(x, y+2*direction).LineTo(x+4, y-2*direction)
		}
		p.StrokePath(&path, 1.6, color)
	})
}
