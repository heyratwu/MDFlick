package svg

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeStaticSVG(t *testing.T) {
	plain := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path fill="red" d="M0 0H10V10Z"/></svg>`)
	got, err := Normalize(context.Background(), plain)
	if err != nil || string(got) != string(plain) {
		t.Fatal("plain markup changed", string(got), err)
	}
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><style>:root{--brand:#f80}</style><path fill="var(--brand)" d="M0 0H10V10Z"/></svg>`)
	got, err = Normalize(context.Background(), source)
	if err != nil || !strings.Contains(string(got), `fill="#f80"`) {
		t.Fatal("variables unresolved", string(got), err)
	}
}
