package main

import (
	"context"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
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
	var result preparedReaderDocument
	if err := ctx.Err(); err != nil {
		return result, err
	}
	parsed := parser.Parse(source)
	result.hasMermaid = document.HasMermaid(parsed)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.markdown, result.ranges = document.NativeHTML(source)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.markdown != source {
		parsed = parser.Parse(result.markdown)
	}
	needsReferences := len(result.ranges) > 0
	for i := 1; !needsReferences && i < parsed.Len(); i++ {
		needsReferences = parsed.Block(i).Kind == parser.Table
	}
	references := ""
	if needsReferences {
		var err error
		references, err = readerReferences(ctx, parsed)
		if err != nil {
			return preparedReaderDocument{}, err
		}
	}
	result.tables = make(map[int]string)
	result.prose = make(map[int]string)
	for i := 1; i < parsed.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return preparedReaderDocument{}, err
		}
		b := parsed.Block(i)
		if b.Kind == parser.Table {
			result.tables[b.Start] = readerTableMarkdown(parsed, i, references)
		}
		k := sort.Search(len(result.ranges), func(k int) bool { return result.ranges[k].End > b.Start })
		aligned := k < len(result.ranges) && result.ranges[k].Start <= b.Start
		if len(result.ranges) > 0 && (b.Kind == parser.Heading || (b.Kind == parser.Paragraph && aligned)) {
			result.prose[b.Start] = result.markdown[b.Start:b.End] + "\n\n" + references
		}
	}
	return result, ctx.Err()
}

func readerTableMarkdown(d *parser.Document, i int, refs string) string {
	b := d.Block(i)
	// Rebuild rows from cell source ranges to remove list/quote prefixes.
	var table strings.Builder
	for row := range d.Children(i) {
		table.WriteByte('|')
		for cell := range d.Children(row) {
			cb := d.Block(cell)
			table.WriteString(d.Source()[cb.Start:cb.End])
			table.WriteByte('|')
		}
		table.WriteByte('\n')
		if d.Block(row).Header {
			table.WriteByte('|')
			for _, align := range b.Align {
				switch align {
				case parser.AlignCenter:
					table.WriteString(":---:|")
				case parser.AlignRight:
					table.WriteString("---:|")
				case parser.AlignLeft:
					table.WriteString(":---|")
				default:
					table.WriteString("---|")
				}
			}
			table.WriteByte('\n')
		}
	}
	return table.String() + "\n" + refs
}

func readerReferences(ctx context.Context, d *parser.Document) (string, error) {
	// Keep reference definitions available to blocks rendered in nested views.
	var refs strings.Builder
	end := 0
	for j := range d.Children(0) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		block := d.Block(j)
		gap := strings.TrimLeft(d.Source()[end:block.Start], " \t|>")
		if parser.Parse(gap).Len() == 1 {
			refs.WriteString(gap)
			refs.WriteByte('\n')
		}
		end = block.End
	}
	if gap := strings.TrimLeft(d.Source()[end:], " \t|>"); parser.Parse(gap).Len() == 1 {
		refs.WriteString(gap)
	}
	return refs.String(), ctx.Err()
}
