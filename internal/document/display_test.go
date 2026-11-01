package document

import (
	"context"
	"testing"

	"mdflick/internal/testfixtures"
)

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
