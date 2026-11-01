package main

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestPreparedReferenceSemantics(t *testing.T) {
	for _, tc := range []struct{ name, text, definitions string }{
		{"full", "[**text**][ref]", "[ref]: https://example.com \"title\""},
		{"collapsed", "[ref][]", "[ref]: https://example.com"},
		{"shortcut", "[ref]", "[REF]: https://example.com"},
		{"image", "![alt][pic]", "[pic]: image.png \"picture\""},
		{"linked-image", "[![alt][pic]][ref]", "[pic]: image.png\n[ref]: https://example.com"},
		{"escaped-label", `[x][a\]b]`, `[a\]b]: https://example.com`},
		{"multiline", "[text][some label]", "[Some   Label]: <https://example.com/a?x=1&y=2>\n  \"multi\nline title\""},
		{"entities", "[ref]", `[ref]: <https://example.com/a&#92;b?x=&quot;&y=&lt;> "title &quot; &amp;"`},
		{"duplicates", "[ref]", "[ref]: https://first.example\n[ref]: https://second.example"},
		{"inline", "[ref](https://inline.example)", "[ref]: https://unused.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.text + "\n\n" + tc.definitions + "\n\n[unused]: https://unused.example\n"
			d := parser.Parse(source)
			prepared, err := prepareReaderDocument(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := prepared.parsed.HTML(), d.HTML(); got != want {
				t.Fatalf("reference changed: got %s want %s", got, want)
			}

		})
	}
}

func TestPreparedReferencesDoNotMultiplyUnusedDefinitions(t *testing.T) {
	source := strings.Repeat("# [Used][r]\n\n", 100) + "[r]: https://example.com\n"
	for i := 0; i < 1000; i++ {
		source += "[unused" + strings.Repeat("x", i%10) + "]: https://unused.example/" + strings.Repeat("a", 100) + "\n"
	}
	prepared, err := prepareReaderDocument(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.markdown != source || prepared.parsed.Source() != source {
		t.Fatal("document source was expanded")
	}
	a := &readerWindow{documentSession: documentSession{path: "/tmp/references.md", source: source, prepared: prepared}}
	tt := ui.NewTester(a.view, 900, 720)
	if !tt.HasText("Used") {
		t.Fatal("reference heading missing")
	}
}
