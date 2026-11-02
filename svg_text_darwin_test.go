//go:build darwin

package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSVGTextTrendshiftBadge(t *testing.T) {
	data, err := os.ReadFile("internal/testfixtures/images/trendshift-badge.svg")
	if err != nil {
		t.Fatal(err)
	}
	source, err := decodeImage(context.Background(), data, true, func(int64) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := source.(*ui.SVG); !ok {
		t.Fatalf("lost vector SVG: %T", source)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(ui.Hex("#eeeef0")).Padding(20).Children(func() { ui.Image(c, source).Width(750).Height(165).Label("Trendshift") })
	}, 800, 210)
	r, ok := tt.Find("Trendshift")
	if !ok {
		t.Fatal("missing badge")
	}
	// The right-hand title region contains no original graphic paths. Checking
	// its painted pixels detects silent text loss, rather than parse success.
	countInk := func(x0, y0, x1, y1 float32) int {
		count := 0
		im := tt.Image()
		for y := int(r.Y + y0*3); y < int(r.Y+y1*3); y++ {
			for x := int(r.X + x0*3); x < int(r.X+x1*3); x++ {
				red, green, blue, _ := im.At(x, y).RGBA()
				if green < 40000 && red < 50000 && blue < 60000 {
					count++
				}
			}
		}
		return count
	}
	if count := countInk(64, 26, 246, 49); count < 500 {
		t.Fatalf("repository title absent: %d ink pixels", count)
	}
	if count := countInk(60, 10, 155, 22); count < 200 {
		t.Fatalf("GITHUB TRENDING absent: %d ink pixels", count)
	}
	if path := os.Getenv("MDFLICK_SVG_TEXT_SCREENSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSVGTextAndTspanPaths(t *testing.T) {
	ctx := context.Background()
	style := svgTextStyle{family: "Arial", size: 16, fill: "#432787", lineHeight: 1.2}
	shaped, err := shapeSVGText(ctx, "SVG 中文 العربية", style)
	if err != nil || shaped.path == "" || shaped.width <= 0 || shaped.ascent <= 0 {
		t.Fatalf("shape=%+v err=%v", shaped, err)
	}
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="250" height="80"><g font-family="Arial" font-size="16" fill="#432787" transform="translate(4,5)"><text x="125" y="24" text-anchor="middle">SVG <tspan font-weight="700">text</tspan></text><text x="10" y="55"><tspan dx="5" dy="2">Second</tspan></text></g></svg>`)
	converted, err := nativeSVGText(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(converted), "<text") || strings.Contains(string(converted), "<tspan") || !strings.Contains(string(converted), "<path") || !strings.Contains(string(converted), `transform="translate(4,5)"`) {
		t.Fatal(string(converted))
	}
	if err := xml.Unmarshal(converted, new(any)); err != nil {
		t.Fatal(err)
	}
	// Conversion preserves all unrelated shape markup byte-for-byte.
	plain := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="20" height="10" fill="red"/></svg>`)
	got, err := nativeSVGText(ctx, plain)
	if err != nil || string(got) != string(plain) {
		t.Fatalf("changed plain SVG: %s %v", got, err)
	}
}

func TestSVGForeignObjectStaticTextOnly(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="140" height="60"><foreignObject x="2" y="3" width="130" height="50" style="font-family:Arial;font-size:12px;color:#432787;line-height:1.5;text-align:center"><div xmlns="http://www.w3.org/1999/xhtml">Two <b>bold</b> words<br/>Next line<script>DO NOT DRAW</script><img src="https://example.com/never-fetch.png"/></div></foreignObject></svg>`)
	converted, err := nativeSVGText(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"foreignObject", "DO NOT DRAW", "never-fetch"} {
		if strings.Contains(string(converted), bad) {
			t.Fatal("active or unsupported content retained", bad)
		}
	}
	if !strings.Contains(string(converted), "clipPath") || !strings.Contains(string(converted), `fill="#432787"`) {
		t.Fatal(string(converted))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeSVGText(ctx, data); err != context.Canceled {
		t.Fatal(err)
	}
	tooMuch := []byte(`<svg><text>` + strings.Repeat("x", maxSVGTextBytes+1) + `</text></svg>`)
	if _, err := nativeSVGText(context.Background(), tooMuch); err == nil {
		t.Fatal("unbounded text accepted")
	}
}

func TestSVGTextConcurrentLoadingAndCancellation(t *testing.T) {
	style := svgTextStyle{family: "Arial", size: 14, fill: "black", lineHeight: 1.2}
	expected, err := shapeSVGText(context.Background(), "Concurrent SVG", style)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 8)
	for range 8 {
		go func() {
			for range 4 {
				outline, err := shapeSVGText(context.Background(), "Concurrent SVG", style)
				if err != nil {
					done <- err
					return
				}
				if outline.path != expected.path || outline.width != expected.width {
					done <- fmt.Errorf("concurrent font outline changed")
					return
				}
			}
			done <- nil
		}()
	}
	for range 8 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	// A cancelled image job must stop even when another job owns Core Text's
	// callback state; it must not retain the gate or corrupt the next load.
	svgTextGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := shapeSVGText(ctx, "Cancelled SVG", style); result <- err }()
	cancel()
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled SVG load waited for native gate")
	}
	<-svgTextGate
	if _, err := shapeSVGText(context.Background(), "Next SVG", style); err != nil {
		t.Fatal(err)
	}
}
