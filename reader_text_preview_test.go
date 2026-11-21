package main

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReaderTextPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.py")
	const source = "print('你好')\r\n# needle\r\n"
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("\uFEFF" + source)
	resources := newApplicationResources()
	load := func(previous string, compare bool) readerLoadResult {
		return loadReaderFile(context.Background(), path, resources, previous, compare)
	}
	result := load("", false)
	if result.err != nil || result.text == nil || result.prepared.parsed != nil || result.source != source {
		t.Fatalf("incorrect preview load: %+v", result)
	}
	state := result.text
	if lang := state.Buffer.Language(); lang == nil || lang.Name != "Python" || state.SoftWrap || !state.ReadOnly {
		t.Fatalf("incorrect editor setup: language=%v, soft wrap=%t, read-only=%t", lang != nil, state.SoftWrap, state.ReadOnly)
	}
	a := &readerWindow{documentSession: documentSession{path: path, source: result.source}, textPreview: state}
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 78})
	if info, ok := tt.AccessByRole(ui.RoleTextField, "script.py"); !ok || info.Bounds.H < 600 {
		t.Fatalf("preview did not fill the reader: %+v", info)
	}
	tt.Type("cannot edit")
	tt.Command("selectAll")
	tt.Command("copy")
	if got := tt.Clipboard(); got != "print('你好')\n# needle\n" {
		t.Fatalf("selected source after typing = %q", got)
	}
	if err := tt.Click("复制文件全文"); err != nil || tt.Clipboard() != source {
		t.Fatalf("whole-file copy = %q, err = %v", tt.Clipboard(), err)
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("needle")
	if matches, _ := state.FindMatches(); matches != 1 || a.find.open {
		t.Fatalf("find opened the wrong reader or found %d matches", matches)
	}
	tt.Key(0, ui.KeyEscape)

	if output := os.Getenv("MDFLICK_SOURCE_SNAPSHOT"); output != "" {
		f, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if unchanged := load(source, true); unchanged.err != nil || unchanged.text != nil || unchanged.source != source {
		t.Fatalf("unchanged save recreated preview: %+v", unchanged)
	}
	write("# Updated\n")
	if updated := load(source, true); updated.err != nil || updated.text == nil || updated.text.Buffer.String() != "# Updated\n" {
		t.Fatalf("changed save did not prepare new source: %+v", updated)
	}
	write("binary\x00data")
	if invalid := load("", false); invalid.err == nil || invalid.text != nil {
		t.Fatal("binary content entered the editor")
	}
	if notes := newTextPreview("notes.txt", "a long line"); !notes.SoftWrap || notes.Buffer.Language() != nil {
		t.Fatal("plain text should soft-wrap without a tokenizer")
	}
}
