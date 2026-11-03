package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func newSVGImageRenderer(win *mygo.Window, pool *diagramPool) svgImageRenderer {
	create := func() (diagramRuntime, error) { return newDiagramWebRuntime(win) }
	return func(ctx context.Context, data []byte, reserve func(int64) error) (ui.ImageSource, error) {
		// Include pool queueing in the deadline, including for local SVG files.
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var charged int64
		reserveResult := func(bytes int64) error {
			// A runtime-owner closure may retry on this job's window. Charge
			// only the largest retained result, not both attempts.
			if bytes <= charged {
				return nil
			}
			if err := reserve(bytes - charged); err != nil {
				return err
			}
			charged = bytes
			return nil
		}
		result := pool.execute(pooledDiagramJob{ctx: ctx, create: create,
			run: func(ctx context.Context, runtime diagramRuntime) diagramResult {
				web, ok := runtime.(*diagramWebRuntime)
				if !ok {
					return diagramResult{err: fmt.Errorf("SVG 浏览器运行时不可用")}
				}
				expression := `window.mdflickRasterizeSVG("` + base64.StdEncoding.EncodeToString(data) + `")`
				return web.renderImage(ctx, expression, reserveResult)
			},
		})
		return result.image, result.err
	}
}
