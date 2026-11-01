package document

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	for _, ext := range []string{".md", ".markdown", ".MD"} {
		path := filepath.Join(dir, "中文 文件"+ext)
		if err := os.WriteFile(path, []byte("\ufeff# 中文\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got != "# 中文\r\n" {
			t.Fatalf("%q %v", got, err)
		}
	}
	if _, err := Load(filepath.Join(dir, "missing.md")); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := Load(filepath.Join(dir, "script.html")); err == nil {
		t.Fatal("unsupported file accepted")
	}
	path := filepath.Join(dir, "directory.md")
	_ = os.Mkdir(path, 0700)
	if _, err := Load(path); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestReadLimits(t *testing.T) {
	for _, source := range []string{string([]byte{0xff}), strings.Repeat("a", MaxSize+1)} {
		if _, err := Read(strings.NewReader(source)); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
	if got, err := Read(strings.NewReader("")); got != "" || err != nil {
		t.Fatalf("empty: %q %v", got, err)
	}
}

func TestCancelledLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadContext(ctx, "missing.md"); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestCancelBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := contextReader{ctx, strings.NewReader(strings.Repeat("a", 128<<10))}
	b := make([]byte, 128<<10)
	n, err := r.Read(b)
	if err != nil || n != 64<<10 {
		t.Fatalf("unbounded read: %d %v", n, err)
	}
	cancel()
	if _, err := r.Read(b); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
