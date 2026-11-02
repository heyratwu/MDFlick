package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var svgVariablePattern = regexp.MustCompile(`var\(\s*(--[a-zA-Z0-9_-]+)\s*(?:,\s*([^()]*))?\)`)
var svgCSSComments = regexp.MustCompile(`(?s)/\*.*?\*/`)

type svgRootVariables struct{ base, dark, light map[string]string }

// MyGo supports static SVG paints and currentColor, but not CSS variables.
// Resolve simple root variables before parsing. A black/light, white/dark
// paint becomes currentColor, so the same cached vector follows the native
// theme without refetching or decoding an image during a UI build.
func nativeSVGVariables(ctx context.Context, data []byte) ([]byte, error) {
	if !bytes.Contains(data, []byte("var(")) {
		return data, nil
	}
	vars := svgRootVariables{map[string]string{}, map[string]string{}, map[string]string{}}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, styleDepth := 0, 0
	var styles strings.Builder
	var rootStyle string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				for _, attr := range token.Attr {
					if attr.Name.Local == "style" {
						rootStyle = attr.Value
					}
				}
			}
			if token.Name.Local == "style" {
				styleDepth = depth
			}
		case xml.EndElement:
			if depth == styleDepth {
				styleDepth = 0
				styles.WriteByte('\n')
			}
			depth--
		case xml.CharData:
			if styleDepth > 0 {
				styles.Write(token)
			}
		}
	}
	vars.stylesheet(svgCSSComments.ReplaceAllString(styles.String(), ""), 0, 0)
	// Inline root declarations take precedence over stylesheet declarations.
	root := svgCustomDeclarations(rootStyle)
	for k, v := range root {
		vars.base[k] = v
		vars.dark[k] = v
		vars.light[k] = v
	}
	source := string(data)
	var out strings.Builder
	cursor := 0
	matches := svgVariablePattern.FindAllStringSubmatchIndex(source, 65537)
	if len(matches) > 65536 {
		return nil, fmt.Errorf("SVG 变量引用过多")
	}
	for _, match := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := source[match[2]:match[3]]
		fallback := ""
		if match[4] >= 0 {
			fallback = strings.TrimSpace(source[match[4]:match[5]])
		}
		light := vars.resolve(name, fallback, false, 0)
		dark := vars.resolve(name, fallback, true, 0)
		value := light
		if svgNeutral(light, false) && svgNeutral(dark, true) {
			value = "currentColor"
		}
		// Unresolved references remain for MyGo's usual SVG fallback behavior.
		if value == "" || strings.ContainsAny(value, "<>\"'&") {
			continue
		}
		extra := (match[0] - cursor) + len(value)
		if extra > maxImageBytes-out.Len() {
			return nil, fmt.Errorf("SVG 变量展开过大")
		}
		out.WriteString(source[cursor:match[0]])
		out.WriteString(value)
		cursor = match[1]
	}
	if len(source)-cursor > maxImageBytes-out.Len() {
		return nil, fmt.Errorf("SVG 变量展开过大")
	}
	out.WriteString(source[cursor:])
	return []byte(out.String()), nil
}
func svgNeutral(value string, white bool) bool {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if white {
		return value == "#fff" || value == "#ffffff" || value == "white" || value == "rgb(255,255,255)"
	}
	return value == "#000" || value == "#000000" || value == "black" || value == "rgb(0,0,0)"
}
func svgCustomDeclarations(source string) map[string]string {
	result := make(map[string]string)
	for decl := range strings.SplitSeq(source, ";") {
		key, value, ok := strings.Cut(decl, ":")
		key = strings.TrimSpace(key)
		if ok && strings.HasPrefix(key, "--") {
			result[key] = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
		}
	}
	return result
}
func (v svgRootVariables) stylesheet(source string, mode, depth int) {
	if depth > 8 {
		return
	}
	for {
		open := strings.IndexByte(source, '{')
		if open < 0 {
			return
		}
		end, n := open+1, 1
		for end < len(source) && n > 0 {
			if source[end] == '{' {
				n++
			}
			if source[end] == '}' {
				n--
			}
			end++
		}
		if n != 0 {
			return
		}
		selector := strings.TrimSpace(source[:open])
		body := source[open+1 : end-1]
		source = source[end:]
		compact := strings.Join(strings.Fields(strings.ToLower(selector)), "")
		if compact == "@media(prefers-color-scheme:dark)" {
			v.stylesheet(body, 1, depth+1)
			continue
		}
		if compact == "@media(prefers-color-scheme:light)" {
			v.stylesheet(body, 2, depth+1)
			continue
		}
		for part := range strings.SplitSeq(selector, ",") {
			part = strings.TrimSpace(part)
			if part != ":root" && part != "svg" {
				continue
			}
			target := v.base
			if mode == 1 {
				target = v.dark
			}
			if mode == 2 {
				target = v.light
			}
			for key, value := range svgCustomDeclarations(body) {
				target[key] = value
			}
			break
		}
	}
}
func (v svgRootVariables) resolve(name, fallback string, dark bool, depth int) string {
	if depth > 16 {
		return ""
	}
	value, ok := v.base[name]
	themed := v.light
	if dark {
		themed = v.dark
	}
	if candidate, exists := themed[name]; exists {
		value, ok = candidate, true
	}
	if !ok {
		value = fallback
	}
	if strings.HasPrefix(value, "var(") {
		match := svgVariablePattern.FindStringSubmatch(value)
		if len(match) > 0 && match[0] == value {
			return v.resolve(match[1], strings.TrimSpace(match[2]), dark, depth+1)
		}
	}
	return value
}
