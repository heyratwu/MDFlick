package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRefreshKeepsPreviewAcrossDuplicateSaveEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	a := &readerWindow{documentSession: documentSession{path: path}}
	active, version := a.beginLoad()
	prepared, err := prepareReaderDocument(active, "# Original")
	if err != nil {
		t.Fatal(err)
	}
	a.completePrepared(version, "# Original", prepared, nil)
	t.Cleanup(a.documentSession.close)
	tt := ui.NewTester(a.view, 900, 720)
	resources := newApplicationResources()
	for i, contents := range []string{"# Updated", "# Updated", "# Updated"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		previous, revision := a.source, a.revision
		ctx, generation := a.beginRefresh()
		for range 3 {
			tt.Frame()
			if tt.HasText("正在读取…") || !tt.HasText(previous[2:]) {
				t.Fatalf("refresh hid the existing preview: %v", tt.Texts())
			}
		}
		source, next, err := loadReaderDocumentIfChanged(ctx, path, resources, previous, true)
		changed := a.completeRefresh(generation, source, next, err)
		if err != nil || changed != (i == 0) {
			t.Fatalf("unexpected refresh result: changed=%v error=%v", changed, err)
		}
		if i > 0 && (next.parsed != nil || a.revision != revision) {
			t.Fatal("duplicate event reparsed or republished unchanged content")
		}
		tt.Frame()
		if !tt.HasText("Updated") || tt.HasText("正在读取…") {
			t.Fatal(tt.Texts())
		}
	}
	if active.Err() != context.Canceled {
		t.Fatal("successful refresh retained obsolete image context")
	}
}

func TestRefreshPreservesActiveContextAndRejectsStaleResults(t *testing.T) {
	s := &documentSession{}
	active, version := s.beginLoad()
	prepared, err := prepareReaderDocument(active, "# Original")
	if err != nil {
		t.Fatal(err)
	}
	s.completePrepared(version, "# Original", prepared, nil)
	defer s.close()
	first, old := s.beginRefresh()
	second, current := s.beginRefresh()
	if first.Err() != context.Canceled || active.Err() != nil {
		t.Fatal("refresh cancelled the displayed document or kept obsolete work")
	}
	if s.completeRefresh(old, "obsolete", prepared, nil) {
		t.Fatal("stale refresh was published")
	}
	if s.completeRefresh(current, "", preparedReaderDocument{}, errors.New("temporary save failure")) || s.failure != "" || s.source != "# Original" || s.prepared.parsed != prepared.parsed || s.loading {
		t.Fatal("transient save failure hid the preview")
	}
	if second.Err() != context.Canceled || active.Err() != nil {
		t.Fatal("failed refresh invalidated the current document lifetime")
	}
	refresh, stale := s.beginRefresh()
	_, manual := s.beginLoad()
	if refresh.Err() != context.Canceled || active.Err() != context.Canceled || s.accepts(stale) || !s.accepts(manual) {
		t.Fatal("manual reload did not supersede refresh work")
	}
}
