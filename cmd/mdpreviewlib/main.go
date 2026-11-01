//go:build darwin && cgo

// mdpreviewlib embeds the Markdown renderer in the sandboxed Quick Look process.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"mdflick/internal/document"
	"mdflick/internal/preview"
	"unsafe"
)

// MDPreviewHTML returns a C-owned UTF-8 string; the caller must free it.
// A nil result means the input exceeds the size limit or is not UTF-8.
//
//export MDPreviewHTML
func MDPreviewHTML(input *C.char, size C.int) *C.char {
	if size < 0 || size > document.MaxSize {
		return nil
	}
	source, err := document.Read(bytes.NewReader(C.GoBytes(unsafe.Pointer(input), size)))
	if err != nil {
		return nil
	}
	return C.CString(preview.HTML(source))
}

func main() {}
