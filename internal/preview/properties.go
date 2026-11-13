package preview

import (
	"html"
	"strings"

	"mdflick/internal/document"
)

// Metadata uses only escaped text and native HTML disclosures. This output is
// independent of the Markdown HTML sanitizer and requires no script.
func propertiesHTML(properties *document.FrontMatter, filename string) string {
	var output strings.Builder
	output.WriteString(`<section class="document-properties"><div class="properties-title">`)
	output.WriteString(document.PropertiesTitle(filename))
	output.WriteString(`</div>`)
	writePropertyRows(&output, properties.Fields)
	output.WriteString(`</section>`)
	return output.String()
}

func writePropertyRows(output *strings.Builder, fields []document.Property) {
	output.WriteString(`<dl>`)
	for _, field := range fields {
		output.WriteString(`<div><dt>`)
		output.WriteString(html.EscapeString(field.Name))
		output.WriteString(`</dt><dd>`)
		if text, inline := field.DisplayText(); inline {
			output.WriteString(html.EscapeString(text))
		} else {
			output.WriteString(`<details><summary>`)
			output.WriteString(field.CollectionLabel())
			output.WriteString(`</summary>`)
			writePropertyRows(output, field.Children)
			output.WriteString(`</details>`)
		}
		output.WriteString(`</dd></div>`)
	}
	output.WriteString(`</dl>`)
}
