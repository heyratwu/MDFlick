package preview

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/testfixtures"
)

func TestPreviewAdapterResourceContract(t *testing.T) {
	for _, tc := range testfixtures.ResourceCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := renderHTML(t, "[Visit]("+tc.Destination+")\n\n![asset]("+tc.Destination+")\n")
			if strings.Contains(result, ` src=`) {
				t.Fatal("preview retained document image source")
			}
			if strings.Contains(result, ` href=`) != tc.Anchor {
				t.Fatal("preview link capability mismatch", result)
			}
		})
	}
}

func TestRenderCancellationAtEachStage(t *testing.T) {
	for _, stage := range []string{"preview.parse", "preview.markdown-html", "preview.dom", "preview.sanitize", "preview.output"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			visited := []string{}
			ctx = diagnostics.WithObserver(ctx, func(event diagnostics.Event) {
				visited = append(visited, event.Stage)
				if event.Stage == stage {
					cancel()
				}
			})
			result, err := Render(ctx, "# Heading\n\n| A |\n|---|\n| value |\n")
			if !errors.Is(err, context.Canceled) || result != "" {
				t.Fatal("cancelled work returned output", err)
			}
			if visited[len(visited)-1] != stage {
				t.Fatal("cancelled work entered later stage", visited)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, "# Old"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if result, err := Render(context.Background(), "# New"); err != nil || !strings.Contains(result, "<h1 id=\"new\">New</h1>") {
		t.Fatal("new request failed after cancellation", err)
	}
}

func TestRenderLimits(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		limit        int
	}{
		{"intermediate", strings.Repeat("<", 100), 128},
		{"final", "# Small", 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := render(context.Background(), tc.source, limits{tc.limit, MaxDiagrams})
			if !errors.Is(err, document.ResourceLimit) || result != "" {
				t.Fatal("output limit ignored", err)
			}
		})
	}
	if result, err := Render(context.Background(), strings.Repeat("paragraph\n\n", MaxBlocks)); !errors.Is(err, document.ResourceLimit) || result != "" {
		t.Fatal("block limit ignored", err)
	}
	result, err := Render(context.Background(), strings.Repeat("```mermaid\nflowchart LR; A-->B\n```\n\n", MaxDiagrams+2))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result, `class="mermaid-source"`) != MaxDiagrams || strings.Count(result, "flowchart LR; A--&gt;B") != MaxDiagrams+2 {
		t.Fatal("diagram overflow did not retain source")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := Render(ctx, "# Late"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestRequestsCancelAndRelease(t *testing.T) {
	var registry Requests
	id := registry.New()
	ctx, ok := registry.Context(id)
	if !ok {
		t.Fatal("new request missing")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("request lacks deadline")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); registry.Cancel(id) }()
	}
	wg.Wait()
	if ctx.Err() != context.Canceled {
		t.Fatal("cancellation not propagated")
	}
	registry.Release(id)
	registry.Release(id)
	if _, ok := registry.Context(id); ok {
		t.Fatal("released request retained")
	}
	next := registry.New()
	defer registry.Release(next)
	nextCtx, _ := registry.Context(next)
	if next == id || nextCtx.Err() != nil {
		t.Fatal("new request inherited old cancellation")
	}
}

func BenchmarkPreviewRender(b *testing.B) {
	for _, tc := range testfixtures.PerformanceCases() {
		b.Run(tc.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.Source)))
			for b.Loop() {
				if _, err := Render(context.Background(), tc.Source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkPreviewCancelAfterParse(b *testing.B) {
	source := testfixtures.PerformanceCases()[0].Source
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		ctx = diagnostics.WithObserver(ctx, func(event diagnostics.Event) {
			if event.Stage == "preview.parse" {
				cancel()
			}
		})
		_, err := Render(ctx, source)
		cancel()
		if !errors.Is(err, context.Canceled) {
			b.Fatal(err)
		}
	}
}

func TestPreviewHeadingAnchors(t *testing.T) {
	result := renderHTML(t, "[Jump](#%E4%B8%AD%E6%96%87-heading)\n\n# 中文 **Heading**!\n\n# 中文 **Heading**!\n")
	if strings.Count(result, `id="中文-heading"`) != 1 || !strings.Contains(result, `href="#%E4%B8%AD%E6%96%87-heading"`) {
		t.Fatal("heading navigation target missing or duplicated", result)
	}
}
