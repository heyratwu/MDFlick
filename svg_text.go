package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const maxSVGTextBytes = 64 << 10

var svgTextElement = regexp.MustCompile(`<(?:[\w.-]+:)?(?:text|foreignObject)(?:[\s/>])`)

type svgTextStyle struct {
	family, fill, color, align, anchor, baseline string
	size, spacing, lineHeight, lineHeightPx      float64
	bold, italic, hidden                         bool
}
type svgTextOutline struct {
	path                   string
	width, ascent, descent float64
}
type svgTextPart struct {
	text string
	node *svgTextNode
}
type svgTextNode struct {
	tag   string
	attrs []xml.Attr
	parts []svgTextPart
}
type svgTextBudget struct{ nodes, text int }

// Convert only text-bearing subtrees, retaining the original bytes of all
// other SVG elements. Thus gradients, namespaces, transforms, and shape styles
// continue to be parsed by MyGo without a lossy full-document serialization.
func nativeSVGText(ctx context.Context, data []byte) ([]byte, error) {
	if !svgTextElement.Match(data) {
		return data, nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Entity = map[string]string{"nbsp": "\u00a0"}
	var stack []svgTextStyle
	base := svgTextStyle{size: 16, fill: "black", color: "black", lineHeight: 1.2}
	var output strings.Builder
	last := 0
	budget := svgTextBudget{}
	sequence := 0
	fingerprint := sha256.Sum256(data)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if len(stack) >= 256 {
				return nil, fmt.Errorf("SVG 文字嵌套过深")
			}
			style := base
			if len(stack) > 0 {
				style = stack[len(stack)-1]
			}
			style = style.with(token.Attr)
			if token.Name.Local == "text" || token.Name.Local == "foreignObject" {
				node, err := readSVGTextNode(ctx, decoder, token, 0, &budget)
				if err != nil {
					return nil, err
				}
				sequence++
				var rendered string
				if !style.hidden {
					if node.tag == "foreignObject" {
						rendered, err = renderSVGForeignText(ctx, node, style, fmt.Sprintf("mdflick-text-%x-%d", fingerprint[:8], sequence))
					} else {
						rendered, err = renderSVGTextNode(ctx, node, style)
					}
				}
				if err != nil {
					return nil, err
				}
				if len(rendered)+before-last > maxImageBytes-output.Len() {
					return nil, fmt.Errorf("SVG 文字展开过大")
				}
				output.Write(data[last:before])
				output.WriteString(rendered)
				last = int(decoder.InputOffset())
			} else {
				stack = append(stack, style)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(data)-last > maxImageBytes-output.Len() {
		return nil, fmt.Errorf("SVG 文字展开过大")
	}
	output.Write(data[last:])
	return []byte(output.String()), nil
}
func readSVGTextNode(ctx context.Context, d *xml.Decoder, start xml.StartElement, depth int, budget *svgTextBudget) (*svgTextNode, error) {
	budget.nodes++
	if depth > 64 || budget.nodes > 16384 {
		return nil, fmt.Errorf("SVG 文字节点过多或嵌套过深")
	}
	node := &svgTextNode{tag: start.Name.Local, attrs: start.Attr}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			child, err := readSVGTextNode(ctx, d, token, depth+1, budget)
			if err != nil {
				return nil, err
			}
			node.parts = append(node.parts, svgTextPart{node: child})
		case xml.CharData:
			budget.text += len(token)
			if budget.text > maxSVGTextBytes {
				return nil, fmt.Errorf("SVG 文字内容过多")
			}
			node.parts = append(node.parts, svgTextPart{text: string(token)})
		case xml.EndElement:
			return node, nil
		}
	}
}
func svgAttr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (s svgTextStyle) with(attrs []xml.Attr) svgTextStyle {
	props := make(map[string]string)
	for _, a := range attrs {
		props[a.Name.Local] = a.Value
	}
	for decl := range strings.SplitSeq(svgAttr(attrs, "style"), ";") {
		name, value, ok := strings.Cut(decl, ":")
		if ok {
			props[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	if value := props["font-family"]; value != "" {
		first, _, _ := strings.Cut(value, ",")
		s.family = strings.Trim(strings.TrimSpace(first), "\"'")
	}
	if value, ok := svgLength(props["font-size"], s.size); ok && value > 0 && value <= 4096 {
		s.size = value
	}
	if value := props["font-weight"]; value != "" {
		weight, _ := strconv.Atoi(value)
		s.bold = value == "bold" || value == "bolder" || weight >= 600
	}
	if value := props["font-style"]; value != "" {
		s.italic = value == "italic" || value == "oblique"
	}
	if value := props["fill"]; value != "" {
		s.fill = value
	}
	if value := props["color"]; value != "" {
		s.color = value
	}
	if value := props["text-align"]; value != "" {
		s.align = value
	}
	if value := props["text-anchor"]; value != "" {
		s.anchor = value
	}
	if value := props["dominant-baseline"]; value != "" {
		s.baseline = value
	}
	if value, ok := svgLength(props["letter-spacing"], s.size); ok {
		s.spacing = value
	}
	if value := props["line-height"]; value != "" {
		if ratio, err := strconv.ParseFloat(value, 64); err == nil && ratio > 0 && ratio < 100 {
			s.lineHeight = ratio
			s.lineHeightPx = 0
		} else if px, ok := svgLength(value, s.size); ok && px > 0 {
			s.lineHeightPx = px
		}
	}
	if props["display"] == "none" || props["visibility"] == "hidden" {
		s.hidden = true
	}
	return s
}
func svgLength(value string, em float64) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	factor := 1.0
	switch {
	case strings.HasSuffix(value, "px"):
		value = strings.TrimSuffix(value, "px")
	case strings.HasSuffix(value, "pt"):
		value = strings.TrimSuffix(value, "pt")
		factor = 96.0 / 72
	case strings.HasSuffix(value, "em"):
		value = strings.TrimSuffix(value, "em")
		factor = em
	case strings.HasSuffix(value, "%"):
		value = strings.TrimSuffix(value, "%")
		factor = em / 100
	}
	n, err := strconv.ParseFloat(value, 64)
	n *= factor
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 1e6
}
func svgNumber(n float64) string { return strconv.FormatFloat(n, 'f', 4, 64) }
func svgCoordinate(attrs []xml.Attr, name string) (float64, bool) {
	value := svgAttr(attrs, name)
	// The first position starts a text chunk; per-character position lists are
	// outside this static text subset.
	values := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(values) == 0 {
		return 0, false
	}
	return svgLength(values[0], 16)
}
func svgTextGroup(attrs []xml.Attr) string {
	var b strings.Builder
	b.WriteString("<g")
	for _, a := range attrs {
		if a.Name.Space != "" || a.Name.Local == "xmlns" {
			continue
		}
		// Geometry is handled by text layout; transform and presentation attributes
		// stay on the parent group, preserving clips, opacity and inherited paints.
		switch a.Name.Local {
		case "x", "y", "dx", "dy", "width", "height":
			continue
		}
		b.WriteByte(' ')
		b.WriteString(a.Name.Local)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(a.Value))
		b.WriteByte('"')
	}
	b.WriteByte('>')
	return b.String()
}
func svgTextPath(outline svgTextOutline, x, y float64, fill string) string {
	if outline.path == "" || fill == "none" {
		return ""
	}
	return `<path fill="` + html.EscapeString(fill) + `" d="` + outline.path + `" transform="translate(` + svgNumber(x) + ` ` + svgNumber(y) + `)"/>`
}

type svgPositionedText struct {
	outline svgTextOutline
	x, y    float64
	fill    string
}

func renderSVGTextNode(ctx context.Context, root *svgTextNode, style svgTextStyle) (string, error) {
	x, _ := svgCoordinate(root.attrs, "x")
	y, _ := svgCoordinate(root.attrs, "y")
	dx, _ := svgCoordinate(root.attrs, "dx")
	dy, _ := svgCoordinate(root.attrs, "dy")
	x += dx
	y += dy
	var body strings.Builder
	var chunk []svgPositionedText
	pathBytes := 0
	pendingSpace, hasText := false, false
	anchor := style.anchor
	startX := x
	flush := func() {
		shift := 0.0
		if anchor == "middle" {
			shift = (x - startX) / 2
		}
		if anchor == "end" {
			shift = x - startX
		}
		for _, part := range chunk {
			body.WriteString(svgTextPath(part.outline, part.x-shift, part.y, part.fill))
		}
		chunk = chunk[:0]
		startX = x
	}
	var visit func(*svgTextNode, svgTextStyle, bool) error
	visit = func(node *svgTextNode, style svgTextStyle, isRoot bool) error {
		if style.hidden || node.tag == "script" || node.tag == "style" {
			return nil
		}
		if !isRoot {
			style = style.with(node.attrs)
			if style.hidden {
				return nil
			}
			nx, hasX := svgCoordinate(node.attrs, "x")
			ny, hasY := svgCoordinate(node.attrs, "y")
			if hasX || hasY {
				flush()
				if hasX {
					x = nx
				}
				if hasY {
					y = ny
				}
				startX = x
				anchor = style.anchor
			}
			dx, _ := svgCoordinate(node.attrs, "dx")
			dy, _ := svgCoordinate(node.attrs, "dy")
			x += dx
			y += dy
		}
		for _, part := range node.parts {
			if part.node != nil {
				if err := visit(part.node, style, false); err != nil {
					return err
				}
				continue
			}
			var normalized strings.Builder
			for _, r := range part.text {
				if unicode.IsSpace(r) {
					pendingSpace = hasText
					continue
				}
				if pendingSpace {
					normalized.WriteByte(' ')
					pendingSpace = false
				}
				normalized.WriteRune(r)
				hasText = true
			}
			text := normalized.String()
			if text == "" {
				continue
			}
			outline, err := shapeSVGText(ctx, text, style)
			if err != nil {
				return err
			}
			baseline := y
			if style.baseline == "middle" || style.baseline == "central" {
				baseline += (outline.ascent - outline.descent) / 2
			}
			if style.baseline == "hanging" || style.baseline == "text-before-edge" {
				baseline += outline.ascent
			}
			chunk = append(chunk, svgPositionedText{outline, x, baseline, style.fill})
			x += outline.width
			pathBytes += len(outline.path)
			if pathBytes > maxImageBytes {
				return fmt.Errorf("SVG 文字轮廓过大")
			}
		}
		return nil
	}
	if err := visit(root, style, true); err != nil {
		return "", err
	}
	flush()
	return svgTextGroup(root.attrs) + body.String() + "</g>", nil
}

type svgForeignWord struct {
	text      string
	style     svgTextStyle
	space, br bool
}

func renderSVGForeignText(ctx context.Context, root *svgTextNode, style svgTextStyle, clipID string) (string, error) {
	width, ok := svgCoordinate(root.attrs, "width")
	if !ok || width <= 0 {
		return "", nil
	}
	height, ok := svgCoordinate(root.attrs, "height")
	if !ok || height <= 0 {
		return "", nil
	}
	x, _ := svgCoordinate(root.attrs, "x")
	y, _ := svgCoordinate(root.attrs, "y")
	var words []svgForeignWord
	var collect func(*svgTextNode, svgTextStyle)
	collect = func(node *svgTextNode, s svgTextStyle) {
		if node != root {
			s = s.with(node.attrs)
		}
		if s.hidden {
			return
		}
		switch node.tag {
		case "script", "style", "iframe", "object", "template", "img", "image":
			return
		case "br":
			words = append(words, svgForeignWord{br: true})
			return
		case "strong", "b":
			s.bold = true
		case "em", "i":
			s.italic = true
		}
		block := node.tag == "div" || node.tag == "p"
		if block && len(words) > 0 {
			words = append(words, svgForeignWord{br: true})
		}
		for _, part := range node.parts {
			if part.node != nil {
				collect(part.node, s)
				continue
			}
			var word strings.Builder
			emit := func() {
				if word.Len() > 0 {
					words = append(words, svgForeignWord{text: word.String(), style: s})
					word.Reset()
				}
			}
			for _, r := range part.text {
				if unicode.IsSpace(r) {
					emit()
					if len(words) == 0 || !words[len(words)-1].space {
						words = append(words, svgForeignWord{space: true, style: s})
					}
				} else {
					word.WriteRune(r)
				}
			}
			emit()
		}
		if block {
			words = append(words, svgForeignWord{br: true})
		}
	}
	collect(root, style)
	var line []svgPositionedText
	var body strings.Builder
	lineWidth, lineHeight, ascent, descent, top := 0.0, 0.0, 0.0, 0.0, 0.0
	align := style.align
	pendingSpace := false
	pathBytes := 0
	flush := func() error {
		if len(line) == 0 {
			pendingSpace = false
			return nil
		}
		shift := 0.0
		if align == "center" {
			shift = (width - lineWidth) / 2
		}
		if align == "right" || align == "end" {
			shift = width - lineWidth
		}
		baseline := top + (lineHeight-ascent-descent)/2 + ascent
		for _, part := range line {
			body.WriteString(svgTextPath(part.outline, part.x+shift, baseline, part.fill))
			if body.Len() > maxImageBytes {
				return fmt.Errorf("SVG 文字轮廓过大")
			}
		}
		top += lineHeight
		lineWidth, lineHeight, ascent, descent = 0, 0, 0, 0
		line = line[:0]
		pendingSpace = false
		return nil
	}
	for _, word := range words {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if word.br {
			if err := flush(); err != nil {
				return "", err
			}
			continue
		}
		if top >= height {
			break
		}
		if word.space {
			pendingSpace = len(line) > 0
			continue
		}
		outline, err := shapeSVGText(ctx, word.text, word.style)
		if err != nil {
			return "", err
		}
		pathBytes += len(outline.path)
		if pathBytes > maxImageBytes {
			return "", fmt.Errorf("SVG 文字轮廓过大")
		}
		space := 0.0
		if pendingSpace {
			gap, err := shapeSVGText(ctx, " ", word.style)
			if err != nil {
				return "", err
			}
			space = gap.width
		}
		if len(line) > 0 && lineWidth+space+outline.width > width {
			if err := flush(); err != nil {
				return "", err
			}
			space = 0
		}
		if top >= height {
			break
		}
		line = append(line, svgPositionedText{outline: outline, x: lineWidth + space, fill: word.style.color})
		lineWidth += space + outline.width
		height := word.style.size * word.style.lineHeight
		if word.style.lineHeightPx > 0 {
			height = word.style.lineHeightPx
		}
		lineHeight = max(lineHeight, height)
		ascent = max(ascent, outline.ascent)
		descent = max(descent, outline.descent)
		if word.style.align != "" {
			align = word.style.align
		}
		pendingSpace = false
	}
	if err := flush(); err != nil {
		return "", err
	}
	// Clip the passive HTML text to the foreignObject's own viewport. A generated
	// clip ID is scoped to this SVG and never comes from HTML URLs or scripts.
	id := clipID
	return svgTextGroup(root.attrs) + `<g transform="translate(` + svgNumber(x) + ` ` + svgNumber(y) + `)"><defs><clipPath id="` + id + `"><rect width="` + svgNumber(width) + `" height="` + svgNumber(height) + `"/></clipPath></defs><g stroke="none" clip-path="url(#` + id + `)">` + body.String() + `</g></g></g>`, nil
}
