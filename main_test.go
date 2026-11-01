package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReaderStates(t *testing.T) {
	opened := 0
	a := &readerWindow{open: func() { opened++ }}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	if !tt.HasText("轻松阅读 Markdown") {
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
	if !manager.assignHome(home, "/tmp/first.md") {
		t.Fatal("welcome window was not reused")
	}
	if len(manager.windows) != 1 || manager.windows["/tmp/first.md"] != home || home.path != "/tmp/first.md" {
		t.Fatal("document should occupy the original window")
	}
	if manager.assignHome(home, "/tmp/second.md") {
		t.Fatal("occupied window consumed a second file")
	}
	if manager.assignHome(nil, "/tmp/second.md") {
		t.Fatal("missing window accepted")
	}
	closed := &readerWindow{documentSession: documentSession{closed: true}}
	manager.windows[""] = closed
	if manager.assignHome(closed, "/tmp/second.md") {
		t.Fatal("closed window accepted")
	}
}
