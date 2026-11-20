package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func isPreviewImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpeg", ".jpg", ".bmp", ".gif", ".webp":
		return true
	}
	return false
}

func isImagePath(path string) bool {
	if isPreviewImagePath(path) {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg", ".svgz", ".avif", ".apng", ".ico", ".icns", ".tif", ".tiff", ".heic", ".heif", ".jxl", ".psd", ".raw":
		return true
	}
	return false
}

const imagePreviewPadding float32 = 24

func previewImageSize(w, h, availableW, availableH float32) (float32, float32) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	scale := min(float32(1), max(0, availableW)/w, max(0, availableH)/h)
	return w * scale, h * scale
}

func (a *readerWindow) imagePreview(c *ui.Context) {
	width, height := c.Size()
	if c.TitleBar().Height != 0 {
		height -= readerTitleBarHeight
	}
	naturalW, naturalH := ui.ImageSize(a.preview)
	w, h := previewImageSize(naturalW, naturalH, width-a.sidebarWidth(c)-2*imagePreviewPadding, height-2*imagePreviewPadding)
	ui.Column(c).Key("image-preview").Grow(1).WidthPercent(100).MinHeight(0).Center().Padding(imagePreviewPadding).Children(func() {
		ui.Image(c, a.preview).Key(a.path).Label("图片预览").Size(w, h).Shrink(0)
	})
}
