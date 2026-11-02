package main

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSVGRootVariablesPaintInBothThemes(t *testing.T) {
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="24" fill="none"><style>
 :root { --primary: #000; }
 @media (prefers-color-scheme: dark) { :root { --primary: #fff; } }
 </style><path d="M0 0H80V24H0Z" fill="var(--primary)"/></svg>`)
	src, err := decodeImage(context.Background(), source, true, func(int64) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		background := "#ffffff"
		if c.Theme().Dark {
			background = "#111111"
		}
		ui.Column(c).Fill().Background(ui.Hex(background)).Children(func() { ui.Image(c, src).Label("themed badge").TextColor(c.Theme().Text) })
	}, 100, 60)
	for _, dark := range []bool{false, true, false} {
		tt.SetDark(dark)
		r, ok := tt.Find("themed badge")
		if !ok {
			t.Fatal("missing SVG")
		}
		red, _, _, _ := tt.Image().At(int(r.X+r.W/2), int(r.Y+r.H/2)).RGBA()
		if dark && red < 40000 || !dark && red > 20000 {
			t.Fatalf("invisible or wrong-color SVG, dark=%v red=%d", dark, red)
		}
	}
}

func TestSVGRootVariableResolution(t *testing.T) {
	for _, tc := range []struct{ name, styles, paint, want string }{
		{"static-brand", `:root{--brand:#f80}`, `var(--brand)`, `#f80`},
		{"alias", `:root{--brand:var(--accent);--accent:red}`, `var(--brand)`, `red`},
		{"fallback", "", `var(--missing, blue)`, `blue`},
		{"cycle", `:root{--a:var(--b);--b:var(--a)}`, `var(--a)`, `var(--a)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><style>` + tc.styles + `</style><path fill="` + tc.paint + `"/></svg>`)
			got, err := nativeSVGVariables(context.Background(), data)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), `fill="`+tc.want+`"`) {
				t.Fatal(string(got))
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeSVGVariables(ctx, []byte(`<svg fill="var(--x)"/>`)); err != context.Canceled {
		t.Fatal(err)
	}
}
