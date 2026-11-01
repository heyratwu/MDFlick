//go:build darwin && cgo

// mdpreviewlib embeds the Markdown renderer in the sandboxed Quick Look process.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"time"
	"unsafe"

	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/preview"
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
func MDPreviewRender(input *C.char, size C.int, output **C.char) C.int {
	id := requests.New()
	defer requests.Release(id)
	return MDPreviewRenderRequest(C.ulonglong(id), input, size, output)
}

var requests preview.Requests

//export MDPreviewNewRequest
func MDPreviewNewRequest() C.ulonglong { return C.ulonglong(requests.New()) }

//export MDPreviewCancelRequest
func MDPreviewCancelRequest(id C.ulonglong) { requests.Cancel(uint64(id)) }

//export MDPreviewReleaseRequest
func MDPreviewReleaseRequest(id C.ulonglong) { requests.Release(uint64(id)) }

//export MDPreviewRequestStatus
func MDPreviewRequestStatus(id C.ulonglong) C.int {
	ctx, ok := requests.Context(uint64(id))
	if !ok {
		return C.int(document.InvalidArgument)
	}
	return C.int(document.CodeOf(ctx.Err()))
}

// MDPreviewRenderRequest borrows input only until return. Cancellation and the
// deadline include time spent waiting for the process-wide rendering slot.
//
//export MDPreviewRenderRequest
func MDPreviewRenderRequest(id C.ulonglong, input *C.char, size C.int, output **C.char) (status C.int) {
	if output == nil {
		return C.int(document.InvalidArgument)
	}
	*output = nil
	defer func() {
		if recover() != nil {
			if *output != nil {
				MDPreviewFree(*output)
				*output = nil
			}
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
	ctx, ok := requests.Context(uint64(id))
	if !ok {
		return C.int(document.InvalidArgument)
	}
	select {
	case renderSlots <- struct{}{}:
	case <-ctx.Done():
		return C.int(document.CodeOf(ctx.Err()))
	}
	defer func() { <-renderSlots }()
	if err := ctx.Err(); err != nil {
		return C.int(document.CodeOf(err))
	}
	started := time.Now()
	source, err := document.Decode(unsafe.Slice((*byte)(unsafe.Pointer(input)), int(size)))
	diagnostics.Record(ctx, "bridge.decode", started, int(size))
	if err != nil {
		return C.int(document.CodeOf(err))
	}
	html, err := preview.Render(ctx, source)
	if err != nil {
		return C.int(document.CodeOf(err))
	}
	started = time.Now()
	*output = C.CString(html)
	diagnostics.Record(ctx, "bridge.output-copy", started, len(html))
	if err := ctx.Err(); err != nil {
		MDPreviewFree(*output)
		*output = nil
		return C.int(document.CodeOf(err))
	}
	return C.int(document.OK)
}

var renderSlots = make(chan struct{}, 1)

//export MDPreviewAllowsNavigation
func MDPreviewAllowsNavigation(input *C.char, size C.int) C.int {
	if input == nil || size < 0 || size > 64<<10 {
		return 0
	}
	source, err := document.Decode(unsafe.Slice((*byte)(unsafe.Pointer(input)), int(size)))
	if err == nil && document.QuickLookPolicy().AllowsPreviewNavigation(source) {
		return 1
	}
	return 0
}

func main() {}
