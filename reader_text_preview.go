package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"mdflick/internal/document"
	"mdflick/internal/typography"
)

func isTextPreviewPath(path string) bool {
	if path == "" || document.IsMarkdownPath(path) || isImagePath(path) || os.IsPathSeparator(path[len(path)-1]) {
		return false
	}
	if editor.LanguageFor(path) != nil {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case "", ".txt", ".text", ".log", ".env", ".example", ".csv", ".tsv", ".rst", ".adoc", ".ps1", ".bat", ".cmd", ".awk", ".lock":
		return true
	}
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case ".gitignore", ".gitattributes", ".dockerignore":
		return true
	}
	return strings.HasPrefix(name, ".env.")
}

func readTextPreview(ctx context.Context, path string) (string, error) {
	source, err := document.LoadTextContext(ctx, path)
	switch {
	case errors.Is(err, document.InvalidUTF8):
		return "", errors.New("请使用 UTF-8 编码的文本文件")
	case errors.Is(err, document.NotRegular):
		return "", errors.New("请选择普通文本文件")
	case err == nil && strings.IndexByte(source, 0) >= 0:
		return "", errors.New("无法预览二进制文件")
	}
	return source, err
}

func newTextPreview(path, source string) *editor.State {
	language := editor.LanguageForText(path, source)
	buffer := editor.NewBuffer(source)
	buffer.SetLanguage(language)
	return &editor.State{
		Buffer: buffer, ReadOnly: true, SoftWrap: language == nil,
		Font: ui.Font{Family: typography.CodeFont, Size: 13}, Label: filepath.Base(path),
		Placeholder: "此文件为空", NoActiveLine: true, NoScrollPastEnd: true,
	}
}

func (a *readerWindow) textPreviewView(c *ui.Context) {
	editor.View(c.Key(a.path), a.textPreview).FillWidth().Grow(1).MinHeight(0).AutoFocus()
}
