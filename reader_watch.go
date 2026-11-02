package main

import (
	"context"
	"errors"
	"io"
	"log"
	"time"

	"github.com/egoist/mygo"
)

// MyGo follows the path through its parent directory, including atomic saves
// and files removed and later recreated. Debounce combines a save's writes.
// Called in the load worker, before reading the initial contents.
func watchReaderFile(ctx context.Context, path string, changed func()) {
	watcher, err := mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{
		Debounce: 150 * time.Millisecond,
		MaxDelay: 500 * time.Millisecond,
	})
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("watch Markdown %q: %v", path, err)
		}
		return
	}
	go consumeReaderFileChanges(ctx, watcher, changed)
}

type readerFileWatcher interface {
	Next(context.Context) ([]mygo.FileEvent, error)
	Close() error
}

func consumeReaderFileChanges(ctx context.Context, watcher readerFileWatcher, changed func()) {
	defer watcher.Close()
	for {
		events, err := watcher.Next(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("watch Markdown: %v", err)
			}
			return
		}
		if len(events) != 0 {
			changed()
		}
	}
}
