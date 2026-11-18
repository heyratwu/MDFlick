package main

import (
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReaderFindNavigatesRenderedText(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{
		path: "/tmp/reader-find.md",
		source: "**K**[ey](https://example.com/private-key) key\n\n" +
			strings.Repeat("A line of filler.\n\n", 35) + "```text\nLast KEY\n```\n",
	}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: readerTitleBarHeight})
	tt.SetDark(false)
	checkStatus := func(want string) {
		t.Helper()
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("expected status %q, got %d/%d", want, a.find.current+1, a.find.count)
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("Key")
	checkStatus("1/3")
	if !findColorVisible(tt, color.RGBA{R: 255, G: 235, B: 101, A: 255}) ||
		!findColorVisible(tt, color.RGBA{R: 255, G: 175, B: 55, A: 255}) {
		t.Fatal("expected both standard and active highlights")
	}
	tt.Key(0, ui.KeyEnter)
	checkStatus("2/3")
	click("下一个匹配项")
	checkStatus("3/3")
	if a.scroll.Y <= 0 || !findColorVisible(tt, color.RGBA{R: 255, G: 175, B: 55, A: 255}) {
		t.Fatal("expected the last match to scroll into view")
	}
	click("下一个匹配项")
	checkStatus("1/3")
	if a.scroll.Y != 0 {
		t.Fatalf("expected the first match to scroll back, got %v", a.scroll.Y)
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Key(ui.Shift, ui.KeyEnter)
	checkStatus("3/3")
	click("上一个匹配项")
	checkStatus("2/3")
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("private-key")
	checkStatus("0/0")
	tt.Key(0, ui.KeyEscape)
	if a.find.open {
		t.Fatal("Escape did not close search")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("KEY")
	checkStatus("1/3")
	click("关闭搜索")
	if a.find.open || findColorVisible(tt, color.RGBA{R: 255, G: 175, B: 55, A: 255}) {
		t.Fatal("closing search did not remove the highlights")
	}
}

func TestReaderFindRevealsMatchInsideWideCodeBlock(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{
		path:   "/tmp/reader-find-code.md",
		source: "```text\n" + strings.Repeat("some filler ", 25) + "target\n```\n",
	}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 480, 360)
	tt.SetDark(false)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("target")
	if _, ok := tt.Find("1/1"); !ok ||
		!findColorVisible(tt, color.RGBA{R: 255, G: 175, B: 55, A: 255}) {
		t.Fatal("expected the match past the right edge to scroll into view")
	}
	if a.scroll.MaxY != 0 || len(findScrollbarRows(tt, color.RGBA{R: 245, G: 129, B: 25, A: 255})) != 0 {
		t.Fatal("expected no vertical markers when the document fits the window")
	}
}

func TestReaderFindKeepsScrollbarAndMarksDocumentPositions(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{
		path: "/tmp/reader-find-scrollbar.md",
		source: "marker " + strings.Repeat("filler words ", 250) + "marker\n\n" +
			strings.Repeat("A line of filler.\n\n", 35) + "last marker\n",
	}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 420)
	tt.SetTitleBar(ui.TitleBar{Height: readerTitleBarHeight})
	tt.SetDark(false)
	tt.Move(450, 20)
	closed := tt.Image().RGBAAt(894, 60)

	tt.Key(ui.Cmd, ui.KeyF)
	if a.scroll.MaxY <= 0 || tt.Image().RGBAAt(894, 60) == closed {
		t.Fatal("expected a visible scrollbar before a query, without hovering the document")
	}
	tt.Press(894, 60)
	tt.Move(894, 125)
	tt.Release(894, 125)
	if a.scroll.Y <= 0 {
		t.Fatal("expected the persistent scrollbar thumb to be draggable")
	}
	tt.Move(450, 20)
	tt.Type("marker")
	normal := color.RGBA{R: 221, G: 166, B: 0, A: 255}
	active := color.RGBA{R: 245, G: 129, B: 25, A: 255}
	before := findScrollbarRows(tt, normal)
	first := findScrollbarRows(tt, active)
	if len(before) != 2 || len(first) != 1 || first[0] >= before[0] || before[0] >= before[1] {
		t.Fatalf("expected three ordered marker positions for wrapped text and a later block, got %v and %v", first, before)
	}
	tt.Key(0, ui.KeyEnter)
	second := findScrollbarRows(tt, active)
	if len(second) != 1 || second[0] != before[0] {
		t.Fatalf("expected the active marker to move to the next line, got %v", second)
	}
	offset := a.scroll.Y
	tt.Scroll(450, 300, 0, 900)
	after := findScrollbarRows(tt, active)
	if a.scroll.Y == offset || len(after) != 1 || after[0] != second[0] {
		t.Fatalf("expected the marker to stay at its document position after scrolling, got %v", after)
	}
	tt.Move(450, 20)
	tt.Key(0, ui.KeyEscape)
	if len(findScrollbarRows(tt, normal)) != 0 || len(findScrollbarRows(tt, active)) != 0 ||
		tt.Image().RGBAAt(894, 60) != closed {
		t.Fatal("expected to hide the search track and markers after closing search")
	}
}

func findScrollbarRows(tt *ui.Tester, want color.RGBA) []int {
	frame := tt.Image()
	x := frame.Rect.Max.X - 6
	var rows []int
	marked := false
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		match := frame.RGBAAt(x, y) == want
		if match && !marked {
			rows = append(rows, y)
		}
		marked = match
	}
	return rows
}

func findColorVisible(tt *ui.Tester, want color.RGBA) bool {
	frame := tt.Image()
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		for x := frame.Rect.Min.X; x < frame.Rect.Max.X; x++ {
			if frame.RGBAAt(x, y) == want {
				return true
			}
		}
	}
	return false
}
