package document

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Keep parsing and the number of disclosure controls bounded independently of
// the Markdown body. Larger headers are left in the body verbatim.
const maxFrontMatterBytes = 64 << 10
const maxFrontMatterNodes = 512
const maxFrontMatterDepth = 16

type FrontMatter struct {
	Fields []Property
}

func PropertiesTitle(filename string) string {
	if filename == "SKILL.md" {
		return "SKILL 信息"
	}
	return "文档属性"
}

type Property struct {
	Name     string
	Text     string
	Children []Property
	List     bool
}

// SplitFrontMatter recognizes a nonempty YAML mapping at the start of a
// document. On failure it returns the original input as Markdown.
func SplitFrontMatter(source string) (*FrontMatter, string) {
	bom := len(source) - len(strings.TrimPrefix(source, "\ufeff"))
	limit := min(len(source), bom+maxFrontMatterBytes)
	prefix := source[bom:limit]
	opening, rest, ok := strings.Cut(prefix, "\n")
	if !ok || !frontMatterFence(opening) {
		return nil, source
	}
	start := bom + len(opening) + 1
	end := start
	for len(rest) > 0 {
		line, next, hasNewline := strings.Cut(rest, "\n")
		if frontMatterFence(line) && (hasNewline || limit == len(source)) {
			after := end + len(line)
			if hasNewline {
				after++
			}
			fields := parseProperties(source[start:end])
			if len(fields) > 0 {
				return &FrontMatter{Fields: fields}, source[after:]
			}
			break
		}
		end += len(line) + 1
		rest = next
	}
	return nil, source
}

func frontMatterFence(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}

func parseProperties(source string) []Property {
	decoder := yaml.NewDecoder(strings.NewReader(source))
	var tree, extra yaml.Node
	if decoder.Decode(&tree) != nil || len(tree.Content) != 1 || tree.Content[0].Kind != yaml.MappingNode ||
		!errors.Is(decoder.Decode(&extra), io.EOF) {
		return nil
	}
	budget := maxFrontMatterNodes
	root, ok := readProperty(tree.Content[0], 0, &budget)
	if !ok {
		return nil
	}
	return root.Children
}

func readProperty(node *yaml.Node, depth int, budget *int) (Property, bool) {
	if depth > maxFrontMatterDepth || *budget == 0 {
		return Property{}, false
	}
	*budget -= 1
	switch node.Kind {
	case yaml.ScalarNode:
		value := node.Value
		if value == "" && node.ShortTag() == "!!null" {
			value = "null"
		}
		return Property{Text: value}, true
	case yaml.AliasNode:
		// Display the reference itself, keeping cycles and alias expansion out
		// of both previews. The original anchors are visible in the YAML view.
		return Property{Text: "*" + node.Value}, true
	case yaml.SequenceNode, yaml.MappingNode:
		return readCollection(node, depth, budget)
	default:
		return Property{}, false
	}
}

func readCollection(node *yaml.Node, depth int, budget *int) (Property, bool) {
	list := node.Kind == yaml.SequenceNode
	step := 2
	empty := "{}"
	if list {
		step, empty = 1, "[]"
	}
	if len(node.Content) == 0 {
		return Property{Text: empty}, true
	}
	if len(node.Content)/step > *budget {
		return Property{}, false
	}
	children := make([]Property, 0, len(node.Content)/step)
	var keys map[[2]string]bool
	if !list {
		keys = make(map[[2]string]bool, cap(children))
	}
	for i := 0; i < len(node.Content); i += step {
		var name string
		if list {
			name = strconv.Itoa(i + 1)
		} else {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return Property{}, false
			}
			identity := [2]string{key.ShortTag(), key.Value}
			if keys[identity] {
				return Property{}, false
			}
			keys[identity] = true
			name = key.Value
		}
		child, ok := readProperty(node.Content[i+step-1], depth+1, budget)
		if !ok {
			return Property{}, false
		}
		child.Name = name
		children = append(children, child)
	}
	return Property{Children: children, List: list}, true
}

// DisplayText returns inline text for scalars and short lists of simple values.
// Larger or nested collections can be presented as a disclosure.
func (p Property) DisplayText() (string, bool) {
	if len(p.Children) == 0 {
		return p.Text, true
	}
	if !p.List || len(p.Children) > 4 {
		return "", false
	}
	var text strings.Builder
	for i, item := range p.Children {
		if len(item.Children) > 0 || item.Text == "" || strings.ContainsAny(item.Text, "\r\n") || text.Len()+len(item.Text) > 100 {
			return "", false
		}
		if i > 0 {
			text.WriteString(" · ")
		}
		text.WriteString(item.Text)
	}
	return text.String(), true
}

func (p Property) CollectionLabel() string {
	count := strconv.Itoa(len(p.Children))
	if p.List {
		return count + " 项"
	}
	return count + " 个字段"
}
