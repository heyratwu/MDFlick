package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/document"
)

// The parser has already resolved duplicate definitions, case folding, escapes
// and multiline titles. Recreate only definitions used by this fragment from
// those resolved inlines, rather than copying all document definitions per view.
func fragmentReferences(ctx context.Context, d *parser.Document, root, remaining int) (string, error) {
	var out strings.Builder
	seen := make(map[string]bool)
	end := d.Block(root).End
	for i := root; i < d.Len(); i++ {
		if i != root && d.Block(i).Start >= end {
			break
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		block := d.Block(i)
		if !strings.Contains(d.Source()[block.Start:block.End], "[") {
			continue
		}
		for _, in := range d.Inlines(i) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if (in.Kind != parser.Link && in.Kind != parser.Image) || in.Auto {
				continue
			}
			raw := d.Source()[in.Start:in.End]
			label := referenceLabel(raw)
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			available := remaining - out.Len() - len(label) - 5 // colon, space, angle brackets, newline
			if in.Title != "" {
				available -= 3
			} // space and quotes
			destSize, err := referenceSize(ctx, in.Dest, available)
			if err != nil {
				return "", err
			}
			if _, err := referenceSize(ctx, in.Title, available-destSize); err != nil {
				return "", err
			}
			// Encode decoded destinations/titles as entities so quotes, angle brackets,
			// backslashes and newlines cannot alter the generated definition syntax.
			definition := label + ": <" + referenceValue(in.Dest) + ">"
			if in.Title != "" {
				definition += " \"" + referenceValue(in.Title) + "\""
			}
			definition += "\n"
			if len(definition) > remaining-out.Len() {
				return "", document.ResourceLimit
			}
			out.WriteString(definition)
		}
	}
	return out.String(), ctx.Err()
}

var referenceEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;",
	"\\", "&#92;", "\n", "&#10;", "\r", "&#13;",
)

func referenceValue(s string) string { return referenceEscaper.Replace(s) }

// A reference ends in [label], [] (collapsed) or [text] (shortcut).
// Inline destinations end in ')' and autolinks are excluded by the caller.
func referenceLabel(raw string) string {
	if strings.HasSuffix(raw, "[]") {
		raw = raw[:len(raw)-2]
	}
	if !strings.HasSuffix(raw, "]") {
		return ""
	}
	depth := 0
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] != '[' && raw[i] != ']' {
			continue
		}
		backslashes := 0
		for j := i - 1; j >= 0 && raw[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 != 0 {
			continue
		}
		if raw[i] == ']' {
			depth++
		} else {
			depth--
			if depth == 0 {
				return raw[i:]
			}
		}
	}
	return ""
}

// Check encoded length before allocating an expanded destination/title. In
// particular an entity-heavy value must not allocate beyond the fragment cap.
func referenceSize(ctx context.Context, value string, remaining int) (int, error) {
	size := 0
	if remaining < 0 {
		return 0, document.ResourceLimit
	}
	for i := 0; i < len(value); i++ {
		if i%(64<<10) == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		switch value[i] {
		case '&':
			size += 5
		case '<', '>':
			size += 4
		case '"':
			size += 6
		case '\\', '\n', '\r':
			size += 5
		default:
			size++
		}
		if size > remaining {
			return 0, document.ResourceLimit
		}
	}
	return size, ctx.Err()
}
