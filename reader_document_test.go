package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/testfixtures"
)

func prepareTestReader(t *testing.T, a *readerWindow) {
	t.Helper()
	var err error
	a.prepared, err = prepareReaderDocument(context.Background(), a.source)
	if err != nil {
		t.Fatal(err)
	}
	a.html = readerHTMLState{}
	a.generation++
	t.Cleanup(a.releaseImages)
}

func testDocumentImages(t *testing.T, ctx context.Context, dir string) func(string) (ui.ImageSource, error) {
	t.Helper()
	s := newImageStore(ctx, filepath.Join(dir, "doc.md"), applicationImages)
	t.Cleanup(s.close)
	return s.load
}

func TestPreparedPublication(t *testing.T) {
	a := &readerWindow{}
	_, old := a.beginLoad()
	_, current := a.beginLoad()
	source := "<div align=center>\n\n[Go][ref]\n\n</div>\n\n| Name |\n|---|\n| [Go][ref] |\n\n[ref]: https://example.com\n\n```mermaid\nflowchart LR; A-->B\n```\n"
	prepared, err := prepareReaderDocument(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.hasMermaid || len(prepared.ranges) == 0 || prepared.parsed == nil {
		t.Fatalf("incomplete snapshot: %+v", prepared)
	}
	if !strings.Contains(prepared.parsed.HTML(), `href="https://example.com"`) {
		t.Fatal("prepared parse lost references")
	}
	if a.completePrepared(old, source, prepared, nil) || a.prepared.markdown != "" {
		t.Fatal("stale snapshot published")
	}
	if !a.completePrepared(current, source, prepared, nil) || a.prepared.markdown != prepared.markdown || a.source != source {
		t.Fatal("current snapshot lost")
	}
	a.close()
	if a.completePrepared(current, "", preparedReaderDocument{}, nil) || a.source != "" {
		t.Fatal("closed window accepted snapshot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareReaderDocument(ctx, source); err != context.Canceled {
		t.Fatal(err)
	}
}

func BenchmarkNativePrepare(b *testing.B) {
	for _, tc := range testfixtures.PerformanceCases() {
		b.Run(tc.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.Source)))
			for b.Loop() {
				if _, err := prepareReaderDocument(context.Background(), tc.Source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
