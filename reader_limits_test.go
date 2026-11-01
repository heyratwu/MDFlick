package main

import (
	"context"
	"testing"
	"time"

	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
)

func TestNativeProcessingLimits(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		change       func(*document.ProcessingLimits)
	}{
		{"input-blocks", "a\n\nb\n\nc", func(l *document.ProcessingLimits) { l.Blocks = 3 }},
		{"converted-blocks", "<div><p>a</p><p>b</p><p>c</p></div>", func(l *document.ProcessingLimits) { l.Blocks = 3 }},
		{"converted-bytes", "<hr><hr><hr><hr>", func(l *document.ProcessingLimits) { l.ConvertedBytes = 18 }},
		{"aggregate-fragments", "# One\n\n# Two", func(l *document.ProcessingLimits) { l.FragmentBytes = 10 }},
		{"table-fragment", "| Column |\n|---|\n| value |", func(l *document.ProcessingLimits) { l.FragmentBytes = 8 }},
		{"reference-expansion", "# [r]\n\n[r]: https://example.com/a-long-destination", func(l *document.ProcessingLimits) { l.FragmentBytes = 16 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := document.NativeLimits()
			tc.change(&limits)
			prepared, err := prepareReaderDocumentWithLimits(context.Background(), tc.source, limits)
			if err != document.ResourceLimit {
				t.Fatalf("expected resource limit, got %v", err)
			}
			if prepared.markdown != "" || len(prepared.prose) != 0 || len(prepared.tables) != 0 {
				t.Fatal("partial document returned")
			}
		})
	}
	limits := document.NativeLimits()
	limits.Timeout = -time.Second
	if _, err := prepareReaderDocumentWithLimits(context.Background(), "# Heading", limits); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
}

func TestNativeCancelAtStageBoundaries(t *testing.T) {
	for _, stage := range []string{"native.parse", "native.html", "native.fragments"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reached := false
			ctx = diagnostics.WithObserver(ctx, func(event diagnostics.Event) {
				if event.Stage == stage {
					reached = true
					cancel()
				}
			})
			result, err := prepareReaderDocument(ctx, "<div align=center>\n\n# [Title][r]\n\n</div>\n\n[r]: https://example.com")
			if !reached || err != context.Canceled || result.markdown != "" || len(result.prose) != 0 {
				t.Fatalf("cancel at %s: %+v %v", stage, result, err)
			}
		})
	}
}
