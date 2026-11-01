package document

import (
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
