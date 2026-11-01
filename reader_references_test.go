package main

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestFragmentReferenceSemantics(t *testing.T) {
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
			refs, err := fragmentReferences(context.Background(), d, 1, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(refs, "unused.example") {
				t.Fatal("copied unused definition", refs)
			}
			got, want := parser.Parse(tc.text+"\n\n"+refs).HTML(), d.HTML()
			if got != want {
				t.Fatalf("reference changed: got %s want %s; definitions: %s", got, want, refs)
			}
		})
	}
}

func TestFragmentReferencesDoNotMultiplyUnusedDefinitions(t *testing.T) {
	source := strings.Repeat("# [Used][r]\n\n", 100) + "[r]: https://example.com\n"
	for i := 0; i < 1000; i++ {
		source += "[unused" + strings.Repeat("x", i%10) + "]: https://unused.example/" + strings.Repeat("a", 100) + "\n"
	}
	prepared, err := prepareReaderDocument(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, fragment := range prepared.prose {
		total += len(fragment)
		if strings.Contains(fragment, "unused.example") {
			t.Fatal("unused definitions copied")
		}
	}
	if total > 10_000 {
		t.Fatal("reference amplification", total)
	}
}
