package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const maxImageBytes = 16 << 20
const maxImagePixels = 16_000_000

// Each document version owns its semaphore and Options cache. SVG stays under
// the byte limit; raster dimensions are inspected before allocating pixels.
func documentImages(ctx context.Context, dir string) func(string) (ui.ImageSource, error) {
	slots := make(chan struct{}, 2)
	return func(source string) (ui.ImageSource, error) {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-slots }()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.Contains(source, "://") {
			address, err := url.Parse(source)
			if err != nil {
				return nil, err
			}
			if address.Scheme == "http" || address.Scheme == "https" {
				return remoteImage(ctx, address.String())
			}
			return nil, os.ErrNotExist
		}
		if strings.HasPrefix(source, "data:") {
			return nil, os.ErrNotExist
		}
		path := strings.TrimPrefix(source, "file:")
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
		return decodeImage(ctx, data, strings.EqualFold(filepath.Ext(path), ".svg"))
	}
}

const imageRequestTimeout = 15 * time.Second

var imageHTTPClient = &http.Client{
	Timeout: imageRequestTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("不支持的图片地址协议")
		}
		if len(via) >= 10 {
			return fmt.Errorf("图片重定向次数过多")
		}
		return nil
	},
}

func remoteImage(ctx context.Context, address string) (ui.ImageSource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片请求失败：HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, fmt.Errorf("图片文件过大")
	}
	data, err := readImage(ctx, resp.Body)
	if err != nil {
		return nil, err
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	isSVG := contentType == "image/svg+xml" || strings.EqualFold(filepath.Ext(resp.Request.URL.Path), ".svg")
	return decodeImage(ctx, data, isSVG)
}

func decodeImage(ctx context.Context, data []byte, isSVG bool) (ui.ImageSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if isSVG {
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

func readImage(ctx context.Context, f io.Reader) ([]byte, error) {
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
