package document

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"mdflick/internal/testfixtures"
)

func TestLocalDocumentPath(t *testing.T) {
	cases := map[string]string{
		"../文档/Some%20Notes.MD#title": "../文档/Some Notes.MD",
		"/docs/intro.markdown":        "/docs/intro.markdown",
		"file:///docs/intro.md":       "/docs/intro.md",
		"file://localhost/intro.md":   "/intro.md",
		"file://example.com/intro.md": "",
		"https://example.com/doc.md":  "",
		"//example.com/doc.md":        "",
		"/%2Fexample.com/doc.md":      "",
		"../doc%00.md":                "",
		"../image.png?name=doc.md":    "",
		"#doc.md":                     "",
	}
	if runtime.GOOS == "windows" {
		cases[`C:\docs\intro.md`] = "C:/docs/intro.md"
		cases["C:/docs/intro.md"] = "C:/docs/intro.md"
		cases["file:///C:/docs/intro.md"] = "C:/docs/intro.md"
		cases[`..\docs\intro.md`] = "../docs/intro.md"
		cases[`\\example.com\doc.md`] = ""
		cases["C:doc.md"] = ""
	}
	for dest, want := range cases {
		if got := LocalDocumentPath(dest); got != filepath.FromSlash(want) {
			t.Errorf("LocalDocumentPath(%q) = %q, want %q", dest, got, want)
		}
		if want != "" && (NativePolicy().Link(dest) != OpenLocalDocument || QuickLookPolicy().Link(dest) != IgnoreLink) {
			t.Errorf("local link policy mismatch for %q", dest)
		}
	}
}

func TestDisplayPolicyContract(t *testing.T) {
	for _, tc := range testfixtures.ResourceCases {
		t.Run(tc.Name, func(t *testing.T) {
			native, quick := NativePolicy(), QuickLookPolicy()
			if native.AllowsImage(tc.Destination) != tc.NativeImage || quick.AllowsImage(tc.Destination) {
				t.Fatal("image capability mismatch")
			}
			if (native.Link(tc.Destination) == OpenExternal) != tc.NativeExternal || (native.Link(tc.Destination) == ScrollToAnchor) != tc.Anchor {
				t.Fatal("native link capability mismatch")
			}
			if (quick.Link(tc.Destination) == ScrollToAnchor) != tc.Anchor || quick.Link(tc.Destination) == OpenExternal {
				t.Fatal("preview link capability mismatch")
			}
		})
	}
	for _, dest := range []string{"about:blank", "about:blank#destination"} {
		if !QuickLookPolicy().AllowsPreviewNavigation(dest) {
			t.Fatal(dest)
		}
	}
	for _, dest := range []string{"about:config", "https://example.com", "file:///tmp/doc.md", "about:blank/other", "about:blank?other"} {
		if QuickLookPolicy().AllowsPreviewNavigation(dest) {
			t.Fatal(dest)
		}
	}
	if CodeOf(context.Canceled) != Cancelled || CodeOf(context.DeadlineExceeded) != TimedOut || CodeOf(ResourceLimit) != ResourceLimit {
		t.Fatal("new bridge codes changed")
	}
}

func TestDecodeOwnsInput(t *testing.T) {
	data := []byte("\xef\xbb\xbf# 中文")
	source, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 'x'
	}
	if source != "# 中文" {
		t.Fatal("decoded text borrows input")
	}
}
