package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
)

// The worker exclusively owns parsed until publication. Afterwards the UI owns
// it: MyGo's lazy inline caches make parser.Document mutable, even when displayed
// as read-only. Never share a published parse with a worker or another window.
type preparedReaderDocument struct {
	markdown   string
	parsed     *parser.Document
	ranges     []document.HTMLAlignment
	images     map[int]document.HTMLImage
	hasMermaid bool
}

func prepareReaderDocument(ctx context.Context, source string) (preparedReaderDocument, error) {
	return prepareReaderDocumentWithLimits(ctx, source, document.NativeLimits())
}

func prepareReaderDocumentWithLimits(parent context.Context, source string, limit document.ProcessingLimits) (preparedReaderDocument, error) {
	ctx, cancel := context.WithTimeout(parent, limit.Timeout)
	defer cancel()
	totalStarted := time.Now()
	defer func() { diagnostics.Record(ctx, "native.prepare", totalStarted, len(source)) }()
	started := time.Now()
	var result preparedReaderDocument
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	if len(source) > document.MaxSize {
		return result, document.TooLarge
	}
	parsed, err := limit.Parse(ctx, source)
	diagnostics.Record(ctx, "native.parse", started, len(source))
	if err != nil {
		return preparedReaderDocument{}, err
	}
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	result.hasMermaid = document.HasMermaid(parsed)
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	started = time.Now()
	converted, err := document.NativeHTMLDocumentContext(ctx, parsed, limit)
	result.markdown, result.ranges, result.images = converted.Markdown, converted.Ranges, converted.Images
	if err != nil {
		return preparedReaderDocument{}, err
	}
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	diagnostics.Record(ctx, "native.html", started, len(result.markdown))
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	if result.markdown != source {
		parsed, err = limit.Parse(ctx, result.markdown)
		if err != nil {
			return preparedReaderDocument{}, err
		}
	}
	started = time.Now()
	// Warm the inline caches in the worker. Rendering may read them on the UI
	// thread only after the whole document has passed cancellation checks.
	for i := 1; i < parsed.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return preparedReaderDocument{}, err
		}
		switch parsed.Block(i).Kind {
		case parser.Paragraph, parser.Heading, parser.TableCell:
			parsed.Inlines(i)
		}
	}
	diagnostics.Record(ctx, "native.inlines", started, len(result.markdown))
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	result.parsed = parsed
	return result, nil
}
