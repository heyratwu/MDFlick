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

// MDPreviewMaxSize exposes the single document byte limit to Swift.
//
//export MDPreviewMaxSize
func MDPreviewMaxSize() C.int { return C.int(document.MaxSize) }

// MDPreviewErrorMessage returns an owned UTF-8 message; release with MDPreviewFree.
//
//export MDPreviewErrorMessage
func MDPreviewErrorMessage(code C.int) *C.char { return C.CString(document.Code(code).Error()) }

// MDPreviewFree releases output and error-message buffers. NULL is safe.
//
//export MDPreviewFree
func MDPreviewFree(ptr *C.char) { C.free(unsafe.Pointer(ptr)) }

// MDPreviewRender returns a stable document.Code, and writes an owned UTF-8
// buffer on success. Input is borrowed only for this call. NULL input is valid
// only for size zero. An output slot is required and cleared even on failure.
//
//export MDPreviewRender
func MDPreviewRender(input *C.char, size C.int, output **C.char) (status C.int) {
	if output == nil {
		return C.int(document.InvalidArgument)
	}
	*output = nil
	defer func() {
		if recover() != nil {
			status = C.int(document.InternalFailure)
		}
	}()
	if size < 0 {
		return C.int(document.InvalidArgument)
	}
	if size > document.MaxSize {
		return C.int(document.TooLarge)
	}
	if input == nil && size != 0 {
		return C.int(document.InvalidArgument)
	}
	source, err := document.Read(bytes.NewReader(C.GoBytes(unsafe.Pointer(input), size)))
	if err != nil {
		return C.int(document.CodeOf(err))
	}
	*output = C.CString(preview.HTML(source))
	return C.int(document.OK)
}

func main() {}
