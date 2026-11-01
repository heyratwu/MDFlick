package document

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxSize = 32 << 20

func Read(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return "", err
	}
	if len(b) > MaxSize {
		return "", fmt.Errorf("文件超过 32 MB，无法预览")
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	if !utf8.Valid(b) {
		return "", fmt.Errorf("请使用 UTF-8 编码的 Markdown 文件")
	}
	return string(b), nil
}

func Load(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
	default:
		return "", fmt.Errorf("仅支持 .md 和 .markdown 文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("请选择普通 Markdown 文件")
	}
	return Read(f)
}
