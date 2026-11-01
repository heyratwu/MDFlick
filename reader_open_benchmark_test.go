package main

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Includes worker preprocessing and the real reader view's first headless
// frame. No disk I/O, window creation, WebKit or GPU presentation is included.
func BenchmarkNativeOpen(b *testing.B) {
	cases := []struct{ name, source string }{
		{"prose-1000", "# Reading\n\n" + strings.Repeat("A paragraph with **emphasis** and a [link](https://example.com).\n\n", 1000)},
		{"tables-100", "# Reading\n\n" + strings.Repeat("| Key | Value |\n|---|---|\n| one | two |\n\n", 100)},
		{"aligned-100", "# Reading\n\n" + strings.Repeat("<div align=center>\n\n## Heading\n\nAligned **paragraph**.\n\n</div>\n\n", 100)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				prepared, err := prepareReaderDocument(context.Background(), tc.source)
				if err != nil {
					b.Fatal(err)
				}
				a := &readerWindow{documentSession: documentSession{path: "/tmp/open-benchmark.md", source: tc.source, prepared: prepared}}
				tt := ui.NewTester(a.view, 900, 720)
				if !tt.HasText("Reading") {
					b.Fatal("missing rendered document")
				}
				a.close()
			}
		})
	}
}
