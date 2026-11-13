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
		// The inline object occupies the first character of the link.
		r, ok := tt.Find("\uFFFC" + tc.label)
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
