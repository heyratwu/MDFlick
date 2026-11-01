package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
)

// preparedReaderDocument is built by the load worker and read only after
// publication. Offsets and fragments all refer to markdown, not the raw source.
type preparedReaderDocument struct {
	markdown   string
	ranges     []document.HTMLAlignment
	tables     map[int]string
	prose      map[int]string
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
	result.markdown, result.ranges, err = document.NativeHTMLContext(ctx, parsed, limit)
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
	remaining := limit.FragmentBytes
	result.tables = make(map[int]string)
	result.prose = make(map[int]string)
	for i := 1; i < parsed.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return preparedReaderDocument{}, err
		}
		b := parsed.Block(i)
		k := sort.Search(len(result.ranges), func(k int) bool { return result.ranges[k].End > b.Start })
		aligned := k < len(result.ranges) && result.ranges[k].Start <= b.Start
		if b.Kind != parser.Table && b.Kind != parser.Heading && !(b.Kind == parser.Paragraph && aligned) {
			continue
		}
		if b.End-b.Start > remaining {
			return preparedReaderDocument{}, document.ResourceLimit
		}
		refs, err := fragmentReferences(ctx, parsed, i, remaining)
		if err != nil {
			return preparedReaderDocument{}, err
		}
		var fragment string
		if b.Kind == parser.Table {
			fragment, err = readerTableMarkdownContext(ctx, parsed, i, refs, remaining)
			if err != nil {
				return preparedReaderDocument{}, err
			}
			result.tables[b.Start] = fragment
		} else {
			size := b.End - b.Start + 2 + len(refs)
			if size > remaining {
				return preparedReaderDocument{}, document.ResourceLimit
			}
			fragment = result.markdown[b.Start:b.End] + "\n\n" + refs
			result.prose[b.Start] = fragment
		}
		remaining -= len(fragment)
	}
	diagnostics.Record(ctx, "native.fragments", started, limit.FragmentBytes-remaining)
	if err := ctx.Err(); err != nil {
		return preparedReaderDocument{}, err
	}
	return result, nil
}

// Rebuild cells without list/quote prefixes, with bounded writes and cancellation.
func readerTableMarkdownContext(ctx context.Context, d *parser.Document, i int, refs string, remaining int) (string, error) {
	b := d.Block(i)
	var table strings.Builder
	var failure error
	write := func(s string) {
		if failure != nil {
			return
		}
		if err := ctx.Err(); err != nil {
			failure = err
			return
		}
		if len(s) > remaining-table.Len() {
			failure = document.ResourceLimit
			return
		}
		table.WriteString(s)
	}
	for row := range d.Children(i) {
		if failure != nil {
			return "", failure
		}
		write("|")
		for cell := range d.Children(row) {
			if failure != nil {
				return "", failure
			}
			cb := d.Block(cell)
			write(d.Source()[cb.Start:cb.End])
			write("|")
		}
		write("\n")
		if d.Block(row).Header {
			write("|")
			for _, align := range b.Align {
				switch align {
				case parser.AlignCenter:
					write(":---:|")
				case parser.AlignRight:
					write("---:|")
				case parser.AlignLeft:
					write(":---|")
				default:
					write("---|")
				}
			}
			write("\n")
		}
	}
	write("\n")
	write(refs)
	if failure != nil {
		return "", failure
	}
	return table.String(), ctx.Err()
}
