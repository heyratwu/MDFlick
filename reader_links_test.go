package main

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReaderLocalDocumentLinks(t *testing.T) {
	base := t.TempDir()
	absolute := filepath.Join(base, "absolute.md")
	linkPath := url.URL{Path: filepath.ToSlash(absolute)}
	source := "# Local links\n\nRead [Relative](../other%20notes.MD) or [Absolute][doc].\n\n[doc]: " + linkPath.EscapedPath()
	a := &readerWindow{documentSession: documentSession{path: filepath.Join(base, "nested", "source.md"), source: source}}
	var opened string
	a.openDocument = func(path string) { opened = path }
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	for _, tc := range []struct{ label, path string }{
		{"Relative", filepath.Join(base, "other notes.MD")},
		{"Absolute", absolute},
	} {
		// MyGo keeps the inline object and link label together with a word joiner.
		r, ok := tt.Find("\uFFFC\u2060" + tc.label)
		if !ok {
			t.Fatalf("missing icon for %s in %q", tc.label, tt.Texts())
		}
		opened = ""
		tt.ClickAt(r.X+3, r.Y+r.H/2)
		if opened != tc.path {
			t.Fatalf("%s opened %q, want %q", tc.label, opened, tc.path)
		}
	}
	r, _ := tt.Find("Local links")
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	if got := tt.Clipboard(); strings.TrimSpace(got) != "Local links\n\nRead Relative or Absolute." {
		t.Fatalf("copied link text = %q", got)
	}
}

func TestReaderLocalSourceLinkOpensInCurrentPane(t *testing.T) {
	base := t.TempDir()
	a := &readerWindow{documentSession: documentSession{
		path: filepath.Join(base, "SKILL.md"), source: "[Script](scripts/my%20script.py) and [Notes](notes.txt)",
	}}
	var opened string
	a.openSibling = func(path string) { opened = path }
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	for _, tc := range []struct{ label, path string }{
		{"Script", filepath.Join(base, "scripts", "my script.py")},
		{"Notes", filepath.Join(base, "notes.txt")},
	} {
		r, ok := tt.Find("\uFFFC\u2060" + tc.label)
		if !ok {
			t.Fatalf("missing local file link for %s", tc.label)
		}
		tt.ClickAt(r.X+3, r.Y+r.H/2)
		if opened != tc.path || !a.sidebar.open {
			t.Fatalf("%s opened %q, sidebar open = %v", tc.label, opened, a.sidebar.open)
		}
	}
	a.path = filepath.Join(base, "scripts", "my script.py")
	a.updateSidebarDirectory()
	if a.sidebar.directory != base {
		t.Fatalf("nested link replaced the Skill directory: %q", a.sidebar.directory)
	}
}

func TestReaderLinksRevealSidebarAncestors(t *testing.T) {
	base := t.TempDir()
	docs := filepath.Join(base, "docs")
	chapter := filepath.Join(docs, "chapter")
	a := &readerWindow{
		documentSession: documentSession{
			path:   filepath.Join(docs, "source.md"),
			source: "[Guide](chapter/deep/guide.md) [Script](chapter/tool.py) [Notes](notes.txt) [Outside](../docs-other/other.md)",
		},
		sidebar: readerSidebar{open: true, directory: docs},
	}
	var sibling, external string
	a.openSibling = func(path string) { sibling = path }
	a.openDocument = func(path string) { external = path }
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	click := func(label string) {
		t.Helper()
		r, ok := tt.Find("\uFFFC\u2060" + label)
		if !ok {
			t.Fatalf("missing link %s", label)
		}
		tt.ClickAt(r.X+3, r.Y+r.H/2)
	}
	for _, tc := range []struct{ label, path string }{
		{"Guide", filepath.Join(chapter, "deep", "guide.md")},
		{"Script", filepath.Join(chapter, "tool.py")},
		{"Notes", filepath.Join(docs, "notes.txt")},
	} {
		click(tc.label)
		if sibling != tc.path || external != "" {
			t.Fatalf("%s routed to current pane %q and another window %q", tc.label, sibling, external)
		}
	}
	for _, path := range []string{chapter, filepath.Join(chapter, "deep")} {
		if folder := a.sidebar.folders[path]; folder == nil || !folder.expanded {
			t.Fatalf("ancestor %q was not expanded", path)
		}
	}
	sibling = ""
	click("Outside")
	if sibling != "" || external != filepath.Join(base, "docs-other", "other.md") {
		t.Fatalf("outside link routed to current pane %q and another window %q", sibling, external)
	}
}
