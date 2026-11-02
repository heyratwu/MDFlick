package svg

import (
	"context"
	"strings"
	"testing"
)

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
