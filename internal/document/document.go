package document

import (
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
	if len(b) > MaxSize {
		return "", TooLarge
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
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
