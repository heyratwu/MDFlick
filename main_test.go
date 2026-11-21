package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestReaderStates(t *testing.T) {
	opened := 0
	a := &readerWindow{open: func() { opened++ }}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	if !tt.HasText("轻松阅读 Markdown 和 Skills") {
		t.Fatal(tt.Texts())
	}
	if tt.HasText("打开 .md 或 .markdown 文件 · ⌘O") {
		t.Fatal("obsolete welcome hint is visible")
	}
	if err := tt.Click("打开文件…"); err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Fatalf("open actions: %d", opened)
	}
	if _, ok := tt.Access("打开目录…"); ok {
		t.Fatal("welcome page still has a separate directory button")
	}
	a.path, a.source = "/tmp/demo.md", "# Hello\n\n**Markdown** document\n\n- item\n"
	prepareTestReader(t, a)
	tt.Frame()
	if !tt.HasText("Hello") {
		t.Fatal(tt.Texts())
	}
	for _, text := range []string{"打开文件…", "demo.md", "只读"} {
		if tt.HasText(text) {
			t.Fatalf("reader still shows toolbar text %q", text)
		}
	}
	a.loading = true
	tt.Frame()
	if !tt.HasText("正在读取…") {
		t.Fatal(tt.Texts())
	}
	a.loading, a.failure = false, "测试错误"
	tt.Frame()
	if !tt.HasText("无法打开文件") {
		t.Fatal(tt.Texts())
	}
	a.failure = ""
	tt.Frame()
	tt.SetDark(true)
	tt.Frame()
	if !tt.HasText("Hello") {
		t.Fatal(tt.Texts())
	}
}

func TestAssignHome(t *testing.T) {
	home := &readerWindow{}
	manager := &application{windows: map[string]*readerWindow{"": home}}
	if !manager.assignHome(home, "/tmp/first.md", false) {
		t.Fatal("welcome window was not reused")
	}
	if len(manager.windows) != 1 || manager.windows["/tmp/first.md"] != home || home.path != "/tmp/first.md" {
		t.Fatal("document should occupy the original window")
	}
	if manager.assignHome(home, "/tmp/second.md", false) {
		t.Fatal("occupied window consumed a second file")
	}
	if manager.assignHome(nil, "/tmp/second.md", false) {
		t.Fatal("missing window accepted")
	}
	closed := &readerWindow{documentSession: documentSession{closed: true}}
	manager.windows[""] = closed
	if manager.assignHome(closed, "/tmp/second.md", false) {
		t.Fatal("closed window accepted")
	}
}

func TestAssignHomeWithMultipleWelcomeWindows(t *testing.T) {
	first, second := &readerWindow{}, &readerWindow{}
	manager := &application{windows: make(map[string]*readerWindow)}
	manager.registerWindow(first, "")
	manager.registerWindow(second, "")

	if !manager.assignHome(second, "/tmp/docs", true) {
		t.Fatal("second welcome window could not open a directory")
	}
	if manager.windows["/tmp/docs"] != second || second.sidebar.root != "/tmp/docs" || !second.sidebar.open {
		t.Fatal("directory should occupy the second window")
	}
	if manager.assignHome(second, "/tmp/second.md", false) {
		t.Fatal("directory window consumed another selection")
	}
	if manager.welcomeWindow() != first || !manager.assignHome(first, "/tmp/first.md", false) {
		t.Fatal("first welcome window should still be available")
	}
	if len(manager.windows) != 2 || manager.windows["/tmp/first.md"] != first {
		t.Fatal("the two windows should remain registered independently")
	}
	if manager.assignHome(&readerWindow{}, "/tmp/unregistered.md", false) {
		t.Fatal("unregistered window consumed a selection")
	}

	first, second = &readerWindow{}, &readerWindow{}
	manager.registerWindow(first, "")
	manager.registerWindow(second, "")
	manager.unregisterWindow(first)
	if manager.welcomeWindow() != second {
		t.Fatal("remaining welcome window was not found")
	}
}

func TestNextWindowBounds(t *testing.T) {
	// A display to the left of the primary display, below its menu bar.
	workArea := mygo.Rectangle{X: -1000, Y: 30, Width: 1000, Height: 700}
	tests := []struct {
		name     string
		previous mygo.Rectangle
		want     mygo.Rectangle
	}{
		{
			name:     "offset",
			previous: mygo.Rectangle{X: -900, Y: 120, Width: 600, Height: 400},
			want:     mygo.Rectangle{X: -860, Y: 160, Width: 600, Height: 400},
		},
		{
			name:     "right and bottom edges",
			previous: mygo.Rectangle{X: -610, Y: 315, Width: 600, Height: 400},
			want:     mygo.Rectangle{X: -600, Y: 330, Width: 600, Height: 400},
		},
		{
			name:     "left and top edges",
			previous: mygo.Rectangle{X: -1100, Y: -40, Width: 600, Height: 400},
			want:     mygo.Rectangle{X: -1000, Y: 30, Width: 600, Height: 400},
		},
		{
			name:     "larger than the work area",
			previous: mygo.Rectangle{X: -1100, Y: 0, Width: 1200, Height: 900},
			want:     workArea,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextWindowBounds(test.previous, workArea); got != test.want {
				t.Errorf("window bounds = %+v, want %+v", got, test.want)
			}
		})
	}
}
