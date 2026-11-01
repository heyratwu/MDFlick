package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"mdflick/internal/document"
	"mdflick/internal/testfixtures"
)

func TestNativeAdapterLinkContract(t *testing.T) {
	for _, tc := range testfixtures.ResourceCases {
		t.Run(tc.Name, func(t *testing.T) {
			link := func(label string) string { return "[" + label + "](" + tc.Destination + ")" }
			a := &readerWindow{documentSession: documentSession{path: "/tmp/contract.md", source: link("OutsideLink") + "\n\n<div align=center>\n\n" + link("AlignedLink") + "\n\n</div>\n\n| Name |\n|---|\n| " + link("TableLink") + " |\n"}}
			visited := ""
			a.onLink = func(dest string) { visited = dest }
			prepareTestReader(t, a)
			tt := ui.NewTester(a.view, 1000, 900)
			for _, label := range []string{"OutsideLink", "AlignedLink", "TableLink"} {
				visited = ""
				// Paragraph bounds span the prose column; click the glyphs
				// instead of the empty center of a left-aligned paragraph.
				if label == "AlignedLink" {
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
				} else {
					r, ok := tt.Find(label)
					if !ok {
						t.Fatal("missing link", label)
					}
					tt.ClickAt(r.X+2, r.Y+r.H/2)
				}
				if (visited != "") != (tc.NativeExternal || tc.Anchor) {
					t.Fatalf("%s routed to %q", label, visited)
				}
			}
		})
	}
}

func TestNativeTableAnchorAndHeadingReference(t *testing.T) {
	a := &readerWindow{documentSession: documentSession{path: "/tmp/anchor.md", source: "| Link |\n|---|\n| [Jump](#destination) |\n\n" + strings.Repeat("Paragraph.\n\n", 35) + "# Destination\n\n# [Referenced][r]\n\n[r]: https://example.com\n"}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 1000, 700)
	before, _ := tt.Find("Destination")
	if err := tt.Click("Jump"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	after, ok := tt.Find("Destination")
	if !ok || before.Y <= 700 || after.Y < 0 || after.Y+after.H > 700.5 || a.html.scrollTo != "" {
		t.Fatalf("table anchor did not scroll: before=%+v after=%+v", before, after)
	}
	a.source = "# [Referenced][r]\n\n[r]: https://example.com\n"
	prepareTestReader(t, a)
	tt.Frame()
	visited := ""
	a.onLink = func(dest string) { visited = dest }
	r, ok := tt.Find("Referenced")
	if !ok {
		t.Fatal("heading reference missing")
	}
	tt.ClickAt(r.X+2, r.Y+r.H/2)
	if visited != "https://example.com" {
		t.Fatal("heading fragment lost reference definitions", visited)
	}
}

func TestSharedHeadingAnchorsMatchNative(t *testing.T) {
	for _, text := range []string{"中文 Heading!", "  A & B  ", "a_b-c", "重复 标题", "123", "a\tb"} {
		if got, want := document.HeadingAnchor(text), markdown.Slug(text); got != want {
			t.Fatalf("anchor %q != native %q", got, want)
		}
	}
}
