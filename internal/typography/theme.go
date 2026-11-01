// Package typography defines the GenOffice default document palette shared by
// the native reader and the HTML Quick Look preview.
package typography

const (
	Font     = "Segoe UI, system-ui, PingFang SC, Microsoft YaHei, sans-serif"
	CodeFont = "SF Mono, Menlo, Consolas, monospace"
)

type Palette struct {
	Text, Secondary, Muted, Background, Border, QuoteBar, Link, CodeBackground string
}

func Colors(dark bool) Palette {
	if dark {
		return Palette{"#e4e4e4", "#a0a0a0", "#a0a0a0", "#1e1e1e", "#3a3a3a", "#555555", "#4a9eff", "#23272e"}
	}
	return Palette{"#242424", "#606366", "#616870", "#ffffff", "#e3e6ea", "#d9d9d9", "#0a69da", "#f6f8fa"}
}
