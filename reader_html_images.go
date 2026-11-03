package main

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"mdflick/internal/document"
)

type readerNativeImage struct {
	image     parser.Inline
	alt, link string
	line      int
}

// Image groups remain in the original document: links and copy/selection use
// the same parse as text. No second Markdown view or framework image cache.
func nativeImageGroup(in []parser.Inline) ([]readerNativeImage, bool) {
	var images []readerNativeImage
	line := 0
	var visit func(int, int, string, int) bool
	visit = func(start, end int, link string, depth int) bool {
		if depth > 128 {
			return false
		}
		for k := start; k < end; k = in[k].Next {
			n := in[k]
			switch n.Kind {
			case parser.Image:
				images = append(images, readerNativeImage{n, parser.PlainText(in[k+1 : n.Next]), link, line})
			case parser.Link:
				if !visit(k+1, n.Next, n.Dest, depth+1) {
					return false
				}
			case parser.Emphasis, parser.Strong, parser.Strikethrough:
				if !visit(k+1, n.Next, link, depth+1) {
					return false
				}
			case parser.Text:
				if strings.TrimSpace(n.Text) != "" {
					return false
				}
			case parser.SoftBreak:
			case parser.LineBreak:
				line++
			case parser.HTML:
				tag := strings.ToLower(strings.TrimSpace(n.Text))
				if tag == "<br>" || tag == "<br/>" || tag == "<br />" {
					line++
				} else {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	ok := visit(0, len(in), "", 0)
	return images, ok && len(images) > 0
}

func (a *readerWindow) nativeHTMLBlock(c *ui.Context, d *parser.Document, i int) bool {
	kind := d.Block(i).Kind
	if kind != parser.Paragraph && kind != parser.Heading {
		return false
	}
	images, ok := nativeImageGroup(d.Inlines(i))
	if !ok {
		return false
	}
	align := ui.Start
	pos := d.Block(i).Start
	k := sort.Search(len(a.prepared.ranges), func(k int) bool { return a.prepared.ranges[k].End > pos })
	if k < len(a.prepared.ranges) && a.prepared.ranges[k].Start <= pos {
		switch a.prepared.ranges[k].Align {
		case "center":
			align = ui.Center
		case "right":
			align = ui.End
		}
	}

	ui.Column(c).WidthPercent(100).Gap(8).Children(func() {
		for start := 0; start < len(images); {
			end := start + 1
			for end < len(images) && images[end].line == images[start].line {
				end++
			}
			ui.Row(c).WidthPercent(100).Wrap().Gap(8).Justify(align).Children(func() {
				for _, im := range images[start:end] {
					a.nativeHTMLImage(c, im)
				}
			})
			start = end
		}
	})
	return true
}

func (a *readerWindow) nativeHTMLImage(c *ui.Context, im readerNativeImage) {
	metadata, hasMetadata := a.prepared.images[im.image.Start]
	source := im.image.Dest
	if hasMetadata {
		source = metadata.SourceForTheme(c.Theme().Dark)
	}
	// Image identity follows its source position across theme and loading frames.
	c.Key(im.image.Start)
	box := ui.Column(c).MaxWidthPercent(100).Shrink(1)
	var src ui.ImageSource
	if a.md.ResolveImage != nil {
		src = a.md.ResolveImage(c, source, box)
	}
	naturalW, naturalH := ui.ImageSize(src)
	_, available, _ := readerWidths(c)
	available = max(1, available-a.blockIndent)
	width, height := nativeImageSize(metadata, naturalW, naturalH, available)
	if width > 0 {
		box.Width(width)
	}
	// The committed placeholder already has author-supplied geometry, so loading
	// and viewport admission do not depend on the size of the alt text.
	if height > 0 {
		box.Height(height)
	}
	box.Children(func() {
		draw := func() {
			if src == nil || im.image.Pending {
				if (metadata.Width > 0 || metadata.WidthPercent > 0) && metadata.Height > 0 {
					nativeImagePlaceholder(c, width, height, im.alt)
					return
				}
				label := im.alt
				if label == "" {
					label = "图片"
				}
				ui.Text(c, label).TextColor(c.Theme().TextMuted).Padding(8).Role(ui.RoleImage).Label(im.alt)
			} else {
				image := ui.Image(c, src).Label(im.alt).MaxWidthPercent(100)
				if width > 0 {
					image.Width(width)
				}
				if height > 0 {
					image.Height(height)
				}
				title := im.image.Title
				if metadata.Title != "" {
					title = metadata.Title
				}
				if title != "" {
					image.Tooltip(title)
				}
			}
		}
		if im.link != "" && document.NativePolicy().Link(im.link) != document.IgnoreLink {
			dest := im.link
			handler := a.md.OnLink
			ui.ButtonBase(c).Role(ui.RoleLink).Label(im.alt).Padding(0).MaxWidthPercent(100).OnClick(func() {
				if handler != nil {
					handler(dest)
				}
			}).Children(draw)
		} else {
			draw()
		}
	})
}

func nativeImagePlaceholder(c *ui.Context, width, height float32, alt string) {
	base, highlight := ui.Hex("#E5E5E5"), ui.Hex("#F2F2F2")
	if c.Theme().Dark {
		base, highlight = ui.Hex("#303030"), ui.Hex("#404040")
	}
	ui.Box(c).Size(width, height).Shrink(0).Role(ui.RoleImage).Label(alt).
		LinearGradient(ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
			{Color: base, At: 0}, {Color: highlight, At: .5}, {Color: base, At: 1},
		}})
}

func nativeImageSize(im document.HTMLImage, naturalW, naturalH, available float32) (float32, float32) {
	w, h := im.Width, im.Height
	if im.WidthPercent > 0 {
		w = available * im.WidthPercent / 100
	}
	if naturalW > 0 && naturalH > 0 {
		switch {
		case w > 0 && h == 0:
			h = w * naturalH / naturalW
		case h > 0 && w == 0:
			w = h * naturalW / naturalH
		case w == 0 && h == 0:
			w, h = naturalW, naturalH
		}
	}
	if w > available {
		if h > 0 {
			h *= available / w
		}
		w = available
	}
	return w, h
}
