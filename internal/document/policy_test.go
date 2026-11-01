package document

import (
	"errors"
	"fmt"
	"mdflick/internal/testfixtures"
	"strings"
	"testing"
)

func TestErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   Code
	}{
		{string([]byte{0xff}), InvalidUTF8},
		{strings.Repeat("a", MaxSize+1), TooLarge},
	} {
		_, err := Read(strings.NewReader(tc.source))
		if !errors.Is(err, tc.want) || CodeOf(fmt.Errorf("wrapped: %w", err)) != tc.want {
			t.Fatalf("wrong code: %v", err)
		}
	}
	if CodeOf(nil) != OK || CodeOf(errors.New("disk failed")) != IOFailure {
		t.Fatal("incorrect error classification")
	}
}
func TestMermaidCorpus(t *testing.T) {
	for _, tc := range testfixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := DetectMermaid(tc.Source); got != (tc.MermaidBlocks > 0) {
				t.Fatalf("detected %v", got)
			}
		})
	}
}
