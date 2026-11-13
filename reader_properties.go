package main

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
	"mdflick/internal/typography"
)

func readerProperties(c *ui.Context, properties *document.FrontMatter, path string, width float32) {
	colors := typography.Colors(c.Theme().Dark)
	box := ui.Column(c).Key("document-properties").Margin(0, 0, 20).Padding(8, 12).
		Radius(8).Border(1, ui.Hex(colors.Border)).Background(ui.Hex(colors.CodeBackground)).
		Font(typography.Font).FontSize(14).TextColor(ui.Hex(colors.Text))
	box.Children(func() {
		ui.Text(c, document.PropertiesTitle(filepath.Base(path))).FontWeight(600).Padding(6, 2)
		ui.Column(c).Padding(4, 2, 6).Gap(6).Selectable().Children(func() {
			readerPropertyRows(c, properties.Fields, width-30, ui.Hex(colors.Muted))
		})
	})
}

func readerPropertyRows(c *ui.Context, fields []document.Property, width float32, muted ui.Color) {
	for i, field := range fields {
		row := ui.Column(c).Key(i).Gap(2)
		labelWidth := min(132, width*.34)
		valueWidth := width
		if width >= 300 {
			row.Row().AlignItems(ui.Start).Gap(12)
			valueWidth -= labelWidth + 12
		}
		row.Children(func() {
			label := ui.Text(c, field.Name).Font(typography.CodeFont).FontSize(12).TextColor(muted)
			if width >= 300 {
				label.Width(labelWidth).Shrink(0).Padding(2, 0, 0)
			}
			ui.Column(c).Width(max(0, valueWidth)).Shrink(1).MinWidth(0).Children(func() {
				if text, inline := field.DisplayText(); inline {
					ui.Text(c, text).LineHeight(1.45)
					return
				}
				readerPropertyDisclosure(c, field.CollectionLabel(), func() {
					ui.Column(c).Padding(0, 0, 0, 10).Gap(4).Children(func() {
						readerPropertyRows(c, field.Children, valueWidth-10, muted)
					})
				})
			})
		})
	}
}

func readerPropertyDisclosure(c *ui.Context, label string, content func()) {
	box := ui.Column(c)
	open := ui.Local(box, "open", func() bool { return false })
	box.Children(func() {
		group := ui.CollapsibleBase(c, open)
		group.Trigger.AlignSelf(ui.Start).AlignItems(ui.Center).Gap(6).Padding(3, 0).Label(label).Children(func() {
			arrow := "▸"
			if *open {
				arrow = "▾"
			}
			ui.Text(c, arrow).FontSize(12)
			ui.Text(c, label).FontSize(13)
		})
		group.Panel(content)
	})
}
