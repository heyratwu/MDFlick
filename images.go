package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

const maxImageBytes = 16 << 20
const maxImagePixels = 16_000_000

// Each document version owns its semaphore and Options cache. SVG stays under
// the byte limit; raster dimensions are inspected before allocating pixels.
func localImages(ctx context.Context, dir string) func(string) (ui.ImageSource, error) {
	slots := make(chan struct{}, 2)
	return func(url string) (ui.ImageSource, error) {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-slots }()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.Contains(url, "://") || strings.HasPrefix(url, "data:") {
			return nil, os.ErrNotExist
		}
		path := strings.TrimPrefix(url, "file:")
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, filepath.FromSlash(path))
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxImageBytes {
			return nil, fmt.Errorf("图片文件过大或不是普通文件")
		}
		// Bound bytes even if the file grows after Stat.
		data, err := readImage(ctx, f)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(filepath.Ext(path), ".svg") {
			return ui.ParseSVG(data)
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxImagePixels {
			return nil, fmt.Errorf("图片尺寸过大")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return ui.DecodeBitmap(data)
	}
}

func readImage(ctx context.Context, f *os.File) ([]byte, error) {
	var data []byte
	buf := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := f.Read(buf)
		if len(data)+n > maxImageBytes {
			return nil, fmt.Errorf("图片文件过大")
		}
		data = append(data, buf[:n]...)
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
