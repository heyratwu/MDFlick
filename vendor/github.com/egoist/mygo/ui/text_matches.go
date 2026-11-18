package ui

import (
	"sort"

	"github.com/egoist/mygo/internal/text"
)

// VisitDocumentText visits display text in build order after Children returns.
// Inline text, including links, is already joined into its paragraph. Control
// labels and explicitly unselectable text are skipped.
func (e Element) VisitDocumentText(visit func(Element, string)) {
	if n := e.node(); n != nil {
		n.visitDocumentText(visit)
	}
}

func (n *node) visitDocumentText(visit func(Element, string)) {
	if n.flags&(flagInvisible|flagInert|flagUnselectable|flagClickable|flagEditable) != 0 {
		return
	}
	if n.kind == kindText {
		if n.text != "" {
			visit(wrapElement(n), n.text)
		}
		return
	}
	for child := n.first; child != nil; child = child.next {
		child.visitDocumentText(visit)
	}
}

type textMatches struct {
	starts          []uint32
	length, current int
	color, active   Color
	topInset        float32
}

// HighlightMatches colors sorted, non-overlapping rune ranges of equal
// length without reshaping the text. current is the active range's index
// or -1. ScrollIntoView on the same element reveals the active range, with
// topInset reserved above it in the nearest vertical scroll container.
func (e Element) HighlightMatches(starts []uint32, length, current int, color, active Color, topInset float32) {
	if n := e.node(); n != nil && len(starts) > 0 && n.kind == kindText && !n.isInline() {
		n.matches = &textMatches{starts, length, current, color, active, topInset}
	}
}

func (p *Painter) textMatchBackgrounds(l *text.Layout, x, y float32, m *textMatches) {
	for li := range l.Lines {
		line := &l.Lines[li]
		first := sort.Search(len(m.starts), func(i int) bool { return int(m.starts[i])+m.length > line.Start })
		if first == len(m.starts) || int(m.starts[first]) >= line.End ||
			!p.visible(Rect{x + line.X, y + line.Y, line.Width, line.Height}, 0) {
			continue
		}
		// Follow shaped clusters so matches keep their positions across
		// wrapped lines, ligatures, and right-to-left runs.
		for i := 0; i < len(line.Glyphs); {
			g := line.Glyphs[i]
			left, right := g.X, g.X+g.Advance
			i++
			for i < len(line.Glyphs) && line.Glyphs[i].Cluster == g.Cluster && line.Glyphs[i].Runes == g.Runes {
				left = min(left, line.Glyphs[i].X)
				right = max(right, line.Glyphs[i].X+line.Glyphs[i].Advance)
				i++
			}
			end := g.Cluster + g.Runes
			for k := first + sort.Search(len(m.starts)-first, func(j int) bool {
				return int(m.starts[first+j])+m.length > g.Cluster
			}); k < len(m.starts) && int(m.starts[k]) < end && g.Runes > 0; k++ {
				start := int(m.starts[k])
				a := float32(max(start, g.Cluster)-g.Cluster) / float32(g.Runes)
				b := float32(min(start+m.length, end)-g.Cluster) / float32(g.Runes)
				if g.RTL {
					a, b = 1-b, 1-a
				}
				color := m.color
				if k == m.current {
					color = m.active
				}
				p.Fill(Rect{x + left + a*(right-left), y + line.Y, (b - a) * (right - left), line.Height}, color, 0)
			}
		}
	}
}
