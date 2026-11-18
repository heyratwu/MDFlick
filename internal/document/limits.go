package document

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui/markdown/parser"
)

const (
	MaxBlocks         = 20_000
	MaxDerivedSize    = 64 << 20
	ProcessingTimeout = 15 * time.Second
)

// ProcessingLimits bounds derived representations separately from input size.
// Parser calls remain synchronous; cancellation is checked at stage boundaries.
type ProcessingLimits struct {
	Blocks, ConvertedBytes int
	Timeout                time.Duration
}

func NativeLimits() ProcessingLimits {
	return ProcessingLimits{Blocks: MaxBlocks, ConvertedBytes: MaxDerivedSize, Timeout: ProcessingTimeout}
}
func QuickLookLimits() ProcessingLimits {
	return ProcessingLimits{Blocks: MaxBlocks, ConvertedBytes: MaxDerivedSize, Timeout: ProcessingTimeout}
}

func (l ProcessingLimits) Parse(ctx context.Context, source string) (*parser.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(source) > l.ConvertedBytes {
		return nil, ResourceLimit
	}
	// Match the reader and Quick Look's supported syntax. MyGo's
	// newer default adds alerts, mentions and issue references.
	d := parser.New(parser.GFM | parser.ExtFootnotes | parser.ExtMath)
	d.Set(source)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.Len() > l.Blocks {
		return nil, ResourceLimit
	}
	return d, nil
}
