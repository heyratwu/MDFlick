package typography

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed document.css
var documentCSS string

func CSS() string {
	variables := func(dark bool) string {
		p := Colors(dark)
		alpha := "14%"
		if dark {
			alpha = "22%"
		}
		return fmt.Sprintf("--text:%s;--secondary:%s;--paper:%s;--border:%s;--quote-bar:%s;--link:%s;--code-bg:%s;--inline-code:rgb(129 139 152 / %s);", p.Text, p.Secondary, p.Background, p.Border, p.QuoteBar, p.Link, p.CodeBackground, alpha)
	}
	return strings.NewReplacer("{{light}}", variables(false), "{{dark}}", variables(true)).Replace(documentCSS)
}
