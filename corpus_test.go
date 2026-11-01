package main

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/testfixtures"
	"testing"
)

func TestNativeCorpus(t *testing.T) {
	for _, tc := range testfixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			a := &readerWindow{documentSession: documentSession{path: "/tmp/corpus.md", source: tc.Source}}
			a.resetMarkdown(context.Background(), a.path)
			// Observe actual MyGo code dispatch, without creating a WKWebView.
			found := make(map[int]bool)
			a.md.Code["mermaid"] = func(c *ui.Context, b *markdown.CodeBlockView) {
				if !b.Open {
					found[b.Index] = true
				}
				markdown.DefaultCodeBlock(c, b)
			}
			prepareTestReader(t, a)
			tt := ui.NewTester(a.view, 900, 720)
			for _, dark := range []bool{false, true} {
				clear(found)
				tt.SetDark(dark)
				clear(found)
				tt.Frame()
				if len(found) != tc.MermaidBlocks {
					t.Fatalf("native dispatch found %d, expected %d per build", len(found), tc.MermaidBlocks)
				}
			}
		})
	}
}
