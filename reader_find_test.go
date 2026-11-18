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
