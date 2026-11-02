//go:build darwin

package svg

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"
)

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

func TestNormalizeVariablesBeforeTextOutlines(t *testing.T) {
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><style>:root{--brand:#f80}</style><text x="0" y="20" fill="var(--brand)">Hello</text></svg>`)
	got, err := Normalize(context.Background(), source)
	if err != nil || strings.Contains(string(got), "<text") || !strings.Contains(string(got), "<path") || !strings.Contains(string(got), `fill="#f80"`) {
		t.Fatal("normalization pipeline changed", string(got), err)
	}
}
