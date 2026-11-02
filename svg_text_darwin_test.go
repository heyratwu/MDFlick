//go:build darwin

package main

import (
	"context"
	"image/png"
	"os"
	"testing"

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
