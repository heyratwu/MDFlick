package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/diagnostics"
	"mdflick/internal/document"
	"mdflick/internal/imagecache"
)

const documentImageLimit = imagecache.DefaultEntryLimit
const applicationImageBudget int64 = 256 << 20

type imageResources = imagecache.Resources
type imageStore = imagecache.Store[ui.ImageSource]

func newImageStore(parent context.Context, path string, resources *imageResources, renderers ...svgImageRenderer) *imageStore {
	config := imagecache.DefaultConfig()
	config.AllowSource = document.NativePolicy().AllowsImage
	return imagecache.New(parent, path, resources, config, func(ctx context.Context, dir, source string, reserve func(int64) error) (ui.ImageSource, error) {
		return loadCachedImage(ctx, dir, source, reserve, renderers...)
	})
}

// Decoding and diagnostics belong to the native adapter, outside cache policy.
func loadCachedImage(ctx context.Context, dir, source string, reserve func(int64) error, renderers ...svgImageRenderer) (ui.ImageSource, error) {
	started := time.Now()
	defer func() {
		diagnostics.Record(ctx, "native.image-load-decode", started, 0)
		diagnostics.Memory("native.image-load-decode")
	}()
	return loadDocumentImage(ctx, dir, source, reserve, renderers...)
}
