package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type testFileWatcher struct {
	batches chan []mygo.FileEvent
	closed  chan struct{}
}

func (w *testFileWatcher) Next(ctx context.Context) ([]mygo.FileEvent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case batch, ok := <-w.batches:
		if !ok {
			return nil, io.EOF
		}
		return batch, nil
	}
}

func (w *testFileWatcher) Close() error { close(w.closed); return nil }

func TestReaderFileChangesReloadAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &testFileWatcher{batches: make(chan []mygo.FileEvent, 8), closed: make(chan struct{})}
	changes := make(chan struct{}, 8)
	go consumeReaderFileChanges(ctx, w, func() { changes <- struct{}{} })
	// One reload per batch, including atomic replacement and reappearance.
	w.batches <- nil
	w.batches <- []mygo.FileEvent{{Op: mygo.FileWrite, Path: "."}}
	w.batches <- []mygo.FileEvent{{Op: mygo.FileRemove, Path: "."}, {Op: mygo.FileCreate, Path: "."}}
	w.batches <- []mygo.FileEvent{{Op: mygo.FileRemove, Path: "."}}
	w.batches <- []mygo.FileEvent{{Op: mygo.FileCreate, Path: "."}}
	for range 4 {
		select {
		case <-changes:
		case <-time.After(time.Second):
			t.Fatal("file change did not request a reload")
		}
	}
	cancel()
	select {
	case <-w.closed:
	case <-time.After(time.Second):
		t.Fatal("window cancellation did not release the watcher")
	}
	if len(changes) != 0 {
		t.Fatal("batch requested more than one reload")
	}
}

func TestReaderFileWatchEndsWithoutReload(t *testing.T) {
	w := &testFileWatcher{batches: make(chan []mygo.FileEvent), closed: make(chan struct{})}
	close(w.batches)
	consumeReaderFileChanges(context.Background(), w, func() { t.Fatal("closed watch reloaded document") })
	select {
	case <-w.closed:
	default:
		t.Fatal("finished watch was not closed")
	}
}

func TestReaderRendersExternallySavedContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	a := &readerWindow{documentSession: documentSession{path: path, source: "# Original"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 900, 720)
	if !tt.HasText("Original") {
		t.Fatal(tt.Texts())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &testFileWatcher{batches: make(chan []mygo.FileEvent, 1), closed: make(chan struct{})}
	reloads := make(chan struct{}, 1)
	go consumeReaderFileChanges(ctx, w, func() { reloads <- struct{}{} })
	resources := newApplicationResources()
	for _, heading := range []string{"Saved", "Replaced"} {
		temporary := filepath.Join(filepath.Dir(path), "save.tmp")
		target := path
		if heading == "Replaced" {
			target = temporary
		}
		if err := os.WriteFile(target, []byte("# "+heading), 0600); err != nil {
			t.Fatal(err)
		}
		if target == temporary {
			if err := os.Rename(temporary, path); err != nil {
				t.Fatal(err)
			}
		}
		w.batches <- []mygo.FileEvent{{Op: mygo.FileWrite, Path: "."}}
		select {
		case <-reloads:
		case <-time.After(time.Second):
			t.Fatal("save did not trigger reload")
		}
		loadCtx, version := a.beginLoad()
		source, prepared, err := loadReaderDocument(loadCtx, path, resources)
		if err != nil || !a.completePrepared(version, source, prepared, err) {
			t.Fatalf("reload failed: %v", err)
		}
		tt.Frame()
		if !tt.HasText(heading) || tt.HasText("Original") {
			t.Fatal(tt.Texts())
		}
	}
	cancel()
	<-w.closed
}
