package main

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarFileIcons(t *testing.T) {
	for _, tc := range []struct {
		name      string
		directory bool
		want      *ui.SVG
	}{
		{"main.go", false, readerCodeIcon},
		{"script.PY", false, readerCodeIcon},
		{"component.tsx", false, readerCodeIcon},
		{"script.ps1", false, readerCodeIcon},
		{"Dockerfile", false, readerCodeIcon},
		{"notes.TXT", false, readerTextIcon},
		{"SKILL.md", false, readerMarkdownIcon},
		{"logo.svg", false, readerImageIcon},
		{"archive.zip", false, readerFileIcon},
		{"folder.txt", true, readerFolderIcon},
	} {
		entry := sidebarEntry{path: tc.name, directory: tc.directory}
		if entry.icon(false) != tc.want {
			t.Errorf("wrong icon for %s", tc.name)
		}
	}
}

func TestSidebarDirectoryEntries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.md", "two.MARKDOWN", "notes.txt", "run.py", "image.png", "data.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Document"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.md"), 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := readSidebarEntries(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Fatalf("entries: %+v", entries)
	}
	for _, entry := range entries {
		want := entry.name != "folder.md" && entry.name != "data.bin"
		if entry.enabled != want {
			t.Fatalf("entry: %+v", entry)
		}
	}
}

func TestSidebarToggleAndNavigation(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/one.md", source: "# Reading\n\n```go\nlong code\n```"}}
	prepareTestReader(t, a)
	entries := []sidebarEntry{{name: "one.md", path: "/tmp/one.md", enabled: true}, {name: "two.md", path: "/tmp/two.md", enabled: true}, {name: "notes.txt", path: "/tmp/notes.txt", enabled: true}, {name: "data.bin", path: "/tmp/data.bin"}, {name: "folder", path: "/tmp/folder", directory: true}}
	selected := ""
	a.openSibling = func(path string) { selected = path }
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
	button, ok := tt.Find("切换侧边栏")
	if !ok || button.X < 78 || button.Y != 6 {
		t.Fatalf("sidebar button: %+v", button)
	}
	if err := tt.Click("切换侧边栏"); err != nil {
		t.Fatal(err)
	}
	a.sidebar.entries, a.sidebar.directory = entries, "/tmp"
	tt.Frame()
	if !a.sidebar.open || !tt.HasText("two.md") {
		t.Fatal("sidebar did not open")
	}
	if info, ok := tt.Access("two.md"); !ok || info.Description != "" {
		t.Fatalf("file row still exposes a path tooltip: %+v", info)
	}
	for _, name := range []string{"data.bin", "folder"} {
		tt.Click(name)
		if selected != "" {
			t.Fatalf("disabled entry opened %q", selected)
		}
	}
	if err := tt.Click("two.md"); err != nil {
		t.Fatal(err)
	}
	if selected != "/tmp/two.md" {
		t.Fatalf("selected %q", selected)
	}
	if err := tt.Click("notes.txt"); err != nil || selected != "/tmp/notes.txt" {
		t.Fatalf("text selection = %q, err = %v", selected, err)
	}
	for _, width := range []int{480, 900, 1600} {
		tt.SetSize(width, 720)
		heading, ok := tt.Find("Reading")
		if !ok || heading.X < min(float32(240), float32(width)*.35) || heading.X+heading.W > float32(width) {
			t.Fatalf("heading at %d: %+v", width, heading)
		}
	}
	tt.SetSize(900, 720)
	if output := os.Getenv("MDFLICK_SIDEBAR_SNAPSHOT"); output != "" {
		f, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		tt.Click("切换侧边栏")
		tt.Frame()
		if a.sidebar.open || tt.HasText("two.md") {
			t.Fatal("sidebar did not close")
		}
		tt.Click("切换侧边栏")
		a.sidebar.entries = entries
		tt.Frame()
	}
	tt.SetDark(true)
	tt.Frame()
}

func TestSidebarDragResizesReader(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/skill/SKILL.md", source: "# Skill"}}
	prepareTestReader(t, a)
	a.sidebar.open = true
	a.sidebar.directory = filepath.Dir(a.path)
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
	dividerX := func() float32 {
		t.Helper()
		info, ok := tt.AccessByRole(ui.RoleSplitter, "调整侧栏宽度")
		if !ok {
			t.Fatal("sidebar divider missing")
		}
		return info.Bounds.X + info.Bounds.W/2
	}
	initial := dividerX()
	headingBefore, _ := tt.Find("Skill")
	tt.Press(initial, 300)
	tt.Move(initial+60, 300)
	if x := dividerX(); x != initial+60 {
		t.Fatalf("divider during drag = %v, want %v", x, initial+60)
	}
	tt.Move(initial+110, 300)
	tt.Release(initial+110, 300)
	resized := dividerX()
	if resized != initial+110 {
		t.Fatalf("divider after drag = %v, want %v", resized, initial+110)
	}
	if heading, ok := tt.Find("Skill"); !ok || heading.X <= headingBefore.X || heading.X+heading.W > 900 {
		t.Fatalf("reader did not reflow: %+v", heading)
	}
	tt.Press(resized, 300)
	tt.Move(850, 300)
	if x := dividerX(); x != 540 {
		t.Fatalf("divider width limit = %v", x)
	}
	tt.CancelPress()
	if x := dividerX(); x != resized {
		t.Fatalf("canceled drag changed the divider to %v", x)
	}
	if err := tt.Click("切换侧边栏"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("切换侧边栏"); err != nil {
		t.Fatal(err)
	}
	if x := dividerX(); x != resized {
		t.Fatalf("reopened sidebar changed the divider to %v", x)
	}
}

func TestSidebarReplacesWindowRegistry(t *testing.T) {
	first := &readerWindow{documentSession: documentSession{path: "/one.md"}}
	second := &readerWindow{documentSession: documentSession{path: "/two.md"}}
	manager := &application{windows: map[string]*readerWindow{"/one.md": first, "/two.md": second}}
	if !manager.replaceWindowPath(first, "/two.md") || first.path != "/two.md" || len(manager.windows) != 2 {
		t.Fatal("failed to retain both windows")
	}
	manager.unregisterWindow(second)
	if len(manager.windows) != 1 {
		t.Fatal("closing other window removed current window")
	}
	if manager.replaceWindowPath(first, "/data.bin") || first.path != "/two.md" {
		t.Fatal("accepted unsupported file")
	}
	if !manager.replaceWindowPath(first, "/notes.txt") || manager.windows["/notes.txt"] != first {
		t.Fatal("text file did not replace the window preview")
	}
	if !manager.replaceWindowPath(first, "/three.md") || manager.windows["/three.md"] != first {
		t.Fatal("window was not rekeyed")
	}
	first.closed = true
	if manager.replaceWindowPath(first, "/four.md") {
		t.Fatal("reopened closed window")
	}
}

func TestSidebarFileSwitchKeepsDirectoryList(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/one.md", source: "# Original"}}
	prepareTestReader(t, a)
	a.sidebar = readerSidebar{open: true, directory: "/tmp", entries: []sidebarEntry{{name: "one.md", path: "/tmp/one.md", enabled: true}, {name: "two.md", path: "/tmp/two.md", enabled: true}}}
	a.openSibling = func(path string) {
		a.path, a.source = path, "# Replacement"
		a.updateSidebarDirectory()
		prepareTestReader(t, a)
	}
	tt := ui.NewTester(a.view, 900, 720)
	before, _ := tt.Find("two.md")
	entries := &a.sidebar.entries[0]
	generation := a.sidebar.generation
	if err := tt.Click("two.md"); err != nil {
		t.Fatal(err)
	}
	a.loading = true
	tt.Frame()
	if !tt.HasText("one.md") || !tt.HasText("two.md") || tt.HasText("正在读取目录…") {
		t.Fatal("file switch cleared directory list")
	}
	if a.sidebar.loading || a.sidebar.generation != generation || entries != &a.sidebar.entries[0] {
		t.Fatal("file switch reloaded directory list")
	}
	after, _ := tt.Find("two.md")
	if before != after {
		t.Fatalf("file row moved: before=%+v after=%+v", before, after)
	}
	a.loading = false
	tt.Frame()
	if !tt.HasText("Replacement") {
		t.Fatal("document content did not change")
	}
}

func TestSidebarFolderExpansionAndSpacing(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"empty", "text-only", "nested", "nested/deeper"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"text-only/notes.txt", "nested/deeper/child.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Child"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := readSidebarEntries(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &readerWindow{documentSession: documentSession{path: filepath.Join(dir, "root.md"), source: "# Root"}}
	prepareTestReader(t, a)
	a.sidebar = readerSidebar{open: true, directory: dir, entries: entries}
	a.openSibling = func(path string) {
		a.path, a.source = path, "# Child"
		a.updateSidebarDirectory()
		prepareTestReader(t, a)
	}
	tt := ui.NewTester(a.view, 900, 720)
	expand := func(name, path string) {
		t.Helper()
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		folder := a.sidebar.folders[path]
		if folder == nil || !folder.expanded {
			t.Fatalf("folder did not expand: %s", name)
		}
		contents, err := readSidebarEntries(path)
		a.completeSidebarFolder(path, a.sidebar.generation, contents, err)
		tt.Frame()
	}
	expand("empty", filepath.Join(dir, "empty"))
	if !tt.HasText("空文件夹") {
		t.Fatal("empty folder has no expanded state")
	}
	expand("text-only", filepath.Join(dir, "text-only"))
	if !tt.HasText("notes.txt") {
		t.Fatal("folder without Markdown did not show its files")
	}
	if err := tt.Click("notes.txt"); err != nil || a.path != filepath.Join(dir, "text-only", "notes.txt") {
		t.Fatalf("nested text was not opened: %q, %v", a.path, err)
	}
	expand("nested", filepath.Join(dir, "nested"))
	expand("deeper", filepath.Join(dir, "nested/deeper"))
	parent, _ := tt.Find("nested")
	child, _ := tt.Find("deeper")
	depth := -1
	for _, row := range a.sidebar.rows() {
		if row.name == "deeper" {
			depth = row.depth
		}
	}
	if depth != 1 || child.Y-parent.Y < 34 {
		t.Fatalf("missing indentation or row margins: parent=%+v child=%+v", parent, child)
	}
	generation := a.sidebar.generation
	if err := tt.Click("child.md"); err != nil {
		t.Fatal(err)
	}
	if a.sidebar.directory != dir || a.sidebar.generation != generation || !tt.HasText("notes.txt") || !tt.HasText("Child") {
		t.Fatal("opening nested file reset directory tree")
	}
	for i := 0; i < 3; i++ {
		tt.Click("nested")
		tt.Frame()
		if tt.HasText("child.md") {
			t.Fatal("collapsed folder still shows descendants")
		}
		tt.Click("nested")
		tt.Frame()
		if !tt.HasText("child.md") {
			t.Fatal("reopening folder lost nested expansion")
		}
	}
	if output := os.Getenv("MDFLICK_TREE_SNAPSHOT"); output != "" {
		f, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	tt.Move(30, 200)
	tt.Frame()
	if err := tt.Click("折叠所有文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("child.md") || tt.HasText("notes.txt") {
		t.Fatal("collapse-all left descendants visible")
	}
	for path, folder := range a.sidebar.folders {
		if folder.expanded {
			t.Fatalf("folder still expanded: %s", path)
		}
	}
	tt.Click("nested")
	tt.Frame()
	if !tt.HasText("deeper") || tt.HasText("child.md") {
		t.Fatal("collapse-all did not reset nested expansion")
	}
}

func TestSidebarPendingSwitchKeepsPreviewAndCancelsObsoleteSelection(t *testing.T) {
	resources := newApplicationResources()
	// Hold preparation at admission so the old preview must remain published.
	for i := 0; i < cap(resources.loads); i++ {
		resources.loads <- struct{}{}
	}
	a := &readerWindow{documentSession: documentSession{path: "/tmp/original.md", source: "# Original"}, resources: resources}
	prepareTestReader(t, a)
	prepared := a.prepared.parsed
	manager := &application{windows: map[string]*readerWindow{a.path: a}, resources: resources}
	tt := ui.NewTester(a.view, 900, 720)
	for _, path := range []string{"/tmp/next.md", "/tmp/last.md"} {
		manager.switchDocument(a, path)
		generation := a.switchGeneration
		tt.Frame()
		if a.loading || a.path != "/tmp/original.md" || a.prepared.parsed != prepared || !tt.HasText("Original") || tt.HasText("正在读取…") {
			t.Fatal("pending sidebar selection replaced the published preview")
		}
		if !a.acceptsSidebarSwitch(generation) {
			t.Fatal("current selection rejected")
		}
		cancelled := false
		cancel := a.switchCancel
		a.switchCancel = func() { cancel(); cancelled = true }
		manager.switchDocument(a, a.path)
		if !cancelled || a.acceptsSidebarSwitch(generation) || a.switchCancel != nil {
			t.Fatal("returning to the displayed file did not cancel the pending selection")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.switchCancel = cancel
	generation := a.switchGeneration
	a.close()
	if ctx.Err() != context.Canceled || a.acceptsSidebarSwitch(generation) {
		t.Fatal("closing window accepted a pending replacement")
	}
}

func TestSidebarRejectsObsoleteFolderRead(t *testing.T) {
	path := "/tmp/old"
	a := &readerWindow{sidebar: readerSidebar{generation: 2, folders: map[string]*sidebarFolder{path: {expanded: true, loading: true}}}}
	a.completeSidebarFolder(path, 1, []sidebarEntry{{name: "stale.md"}}, nil)
	if a.sidebar.folders[path].loaded {
		t.Fatal("published folder read from old tree")
	}
	a.closed = true
	a.completeSidebarFolder(path, 2, []sidebarEntry{{name: "stale.md"}}, nil)
	if a.sidebar.folders[path].loaded {
		t.Fatal("published folder read after close")
	}
}
