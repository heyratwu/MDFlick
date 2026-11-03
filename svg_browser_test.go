package main

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func TestSVGUsesBrowserThroughImageStore(t *testing.T) {
	for _, name := range []string{"gitcode-star-badge.svg", "discord-badge.svg", "trendshift-badge.svg", "simple-shapes.svg"} {
		t.Run(name, func(t *testing.T) {
			dir, err := filepath.Abs("internal/testfixtures/images")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			render := svgImageRenderer(func(ctx context.Context, data []byte, reserve func(int64) error) (ui.ImageSource, error) {
				calls++
				if len(data) == 0 {
					t.Fatal("missing SVG source")
				}
				if err := reserve(69 * 20 * 4 * 4); err != nil {
					return nil, err
				}
				b := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 138, 40)))
				b.SetLogicalSize(69, 20)
				return b, nil
			})
			store := newImageStore(context.Background(), filepath.Join(dir, "doc.md"), imageTestResources(1<<20, 2), render)
			defer store.Close()
			first, err := store.Load(name)
			if err != nil {
				t.Fatal(err)
			}
			second, err := store.Load(name)
			if err != nil || first != second || calls != 1 {
				t.Fatal("browser results not cached", err, calls)
			}
			tt := ui.NewTester(func(c *ui.Context) { ui.Row(c).Children(func() { ui.Image(c, first).Label("badge") }) }, 200, 80)
			box, ok := tt.Find("badge")
			if !ok || box.W != 69 || box.H != 20 {
				t.Fatal("Retina pixels changed layout", box)
			}
			options := &markdown.Options{ResolveImage: func(*ui.Context, string, ui.Element) ui.ImageSource { return first }}
			md := ui.NewTester(func(c *ui.Context) { markdown.View(c, "![badge](badge.svg)", options) }, 200, 100)
			box, ok = md.Find("badge")
			if !ok || box.W != 69 || box.H != 20 {
				t.Fatal("Markdown ignored logical bitmap size", box)
			}

		})
	}
}

func TestAllSVGUsesBrowserAndRespectsBudget(t *testing.T) {
	simple := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`)
	rich, err := os.ReadFile("internal/testfixtures/images/discord-badge.svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{simple, rich} {
		calls := 0
		failure := errors.New("budget rejected")
		render := svgImageRenderer(func(ctx context.Context, source []byte, reserve func(int64) error) (ui.ImageSource, error) {
			calls++
			if string(source) != string(data) {
				t.Fatal("SVG rewritten before browser rendering")
			}
			return nil, reserve(4096)
		})
		if _, err := decodeImage(context.Background(), data, true, func(int64) error { return failure }, render); !errors.Is(err, failure) || calls != 1 {
			t.Fatal("browser dispatch or budget rejection lost", err, calls)
		}
		if _, err := decodeImage(context.Background(), data, true, func(int64) error { return nil }); err == nil {
			t.Fatal("SVG silently fell back without browser renderer")
		}
	}
}

func TestBrowserPoolRunsSVGTaskAndCancels(t *testing.T) {
	pool := newDiagramPool(nil)
	defer pool.close()
	runtime := &testDiagramRuntime{closed: make(chan struct{}), renderFn: func(context.Context, diagramKey) diagramResult {
		t.Error("SVG dispatched as Mermaid")
		return diagramResult{}
	}}
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan diagramResult, 1)
	go func() {
		done <- pool.execute(pooledDiagramJob{ctx: ctx, create: func() (diagramRuntime, error) { return runtime, nil }, run: func(ctx context.Context, _ diagramRuntime) diagramResult {
			close(started)
			<-ctx.Done()
			return diagramResult{err: ctx.Err()}
		}})
	}()
	receiveDiagram(t, started)
	cancel()
	if result := receiveDiagram(t, done); !errors.Is(result.err, context.Canceled) {
		t.Fatal(result.err)
	}
	receiveDiagram(t, runtime.closed)
}
