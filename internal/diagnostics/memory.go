package diagnostics

import (
	"log"
	"os"
	"runtime/metrics"
)

func Enabled() bool { return os.Getenv("MDFLICK_TRACE") == "1" }

// Memory is an on-demand Go heap sample, not RSS or WebKit/GPU memory. It
// neither forces a collection nor starts a polling goroutine.
func Memory(stage string) {
	if !Enabled() {
		return
	}
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/total:bytes"}, {Name: "/memory/classes/heap/released:bytes"}}
	metrics.Read(samples)
	log.Printf("memory stage=%s pid=%d heap_objects=%d go_retained=%d", stage, os.Getpid(), samples[0].Value.Uint64(), samples[1].Value.Uint64()-samples[2].Value.Uint64())
}
