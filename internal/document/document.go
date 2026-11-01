package document

import (
	"bytes"
	"context"
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
	return Decode(b)
}

// Decode validates borrowed bytes and returns an owned UTF-8 string. The bridge
// uses it directly to avoid copying C input into intermediate byte buffers.
func Decode(b []byte) (string, error) {
	if len(b) > MaxSize {
		return "", TooLarge
	}
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(b) {
		return "", InvalidUTF8
	}
	return string(b), nil
}

func Load(path string) (string, error) { return LoadContext(context.Background(), path) }

// LoadContext checks cancellation between reads; an active OS read cannot be interrupted.
func LoadContext(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
	default:
		return "", UnsupportedExtension
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
		return "", NotRegular
	}
	return Read(contextReader{ctx, f})
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > 64<<10 {
		p = p[:64<<10]
	}
	return r.reader.Read(p)
}
