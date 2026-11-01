package main

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
)

const (
	documentImageBudget    int64 = 128 << 20
	applicationImageBudget int64 = 256 << 20
	documentImageLimit           = 128
)

var errImageBudget = errors.New("图片缓存已达上限，重新载入可重试")

// Account for active document caches. This is an estimate, not a limit on total
// process/GPU memory. No eviction: MyGo Options also retain image references.
type imageResources struct {
	mu          sync.Mutex
	used, limit int64
	slots       chan struct{}
}

type imageEntry struct {
	done  chan struct{}
	image ui.ImageSource
	err   error
}

type imageStore struct {
	ctx         context.Context
	cancel      context.CancelFunc
	dir         string
	resources   *imageResources
	slots       chan struct{}
	mu          sync.Mutex
	entries     map[string]*imageEntry
	used, limit int64
	maxEntries  int
	closed      bool
}

func newImageStore(parent context.Context, path string, resources *imageResources) *imageStore {
	ctx, cancel := context.WithCancel(parent)
	s := &imageStore{ctx: ctx, cancel: cancel, dir: filepath.Dir(path), resources: resources,
		slots: make(chan struct{}, 2), entries: make(map[string]*imageEntry),
		limit: documentImageBudget, maxEntries: documentImageLimit}
	context.AfterFunc(ctx, s.close)
	return s
}

func (s *imageStore) close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.resources.mu.Lock()
	s.resources.used -= s.used
	s.resources.mu.Unlock()
	s.used = 0
	clear(s.entries)
}

// Normalize local paths and URL fragments before deduplicating. Query strings
// remain significant; redirects are handled by the HTTP client.
func (s *imageStore) key(source string) (string, error) {
	if strings.Contains(source, "://") {
		u, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", errors.New("不支持的图片地址协议")
		}
		u.Fragment = ""
		u.Host = strings.ToLower(u.Host)
		return u.String(), nil
	}
	if strings.HasPrefix(source, "data:") {
		return "", errors.New("不支持的图片地址协议")
	}
	path := strings.TrimPrefix(source, "file:")
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.dir, filepath.FromSlash(path))
	}
	return filepath.Clean(path), nil
}

func acquireImageSlot(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *imageStore) load(source string) (ui.ImageSource, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if !document.NativePolicy().AllowsImage(source) {
		return nil, errors.New("不支持的图片地址协议")
	}
	key, err := s.key(source)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, context.Canceled
	}
	if entry := s.entries[key]; entry != nil {
		s.mu.Unlock()
		select {
		case <-entry.done:
			if err := s.ctx.Err(); err != nil {
				return nil, err
			}
			return entry.image, entry.err
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		}
	}
	if len(s.entries) >= s.maxEntries {
		s.mu.Unlock()
		return nil, errImageBudget
	}
	entry := &imageEntry{done: make(chan struct{})}
	s.entries[key] = entry
	s.mu.Unlock()

	image, err := s.loadOnce(key)
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		image, err = nil, context.Canceled
	}
	entry.image, entry.err = image, err
	close(entry.done)
	s.mu.Unlock()
	return image, err
}

func (s *imageStore) loadOnce(key string) (ui.ImageSource, error) {
	if err := acquireImageSlot(s.ctx, s.slots); err != nil {
		return nil, err
	}
	defer func() { <-s.slots }()
	if err := acquireImageSlot(s.ctx, s.resources.slots); err != nil {
		return nil, err
	}
	defer func() { <-s.resources.slots }()
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	var reserved int64
	reserve := func(bytes int64) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.ctx.Err() != nil {
			return context.Canceled
		}
		s.resources.mu.Lock()
		defer s.resources.mu.Unlock()
		if bytes > s.limit-s.used || bytes > s.resources.limit-s.resources.used {
			return errImageBudget
		}
		s.used += bytes
		s.resources.used += bytes
		reserved += bytes
		return nil
	}
	image, err := loadDocumentImage(s.ctx, s.dir, key, reserve)
	if err == nil {
		err = s.ctx.Err()
	}
	if err != nil {
		s.mu.Lock()
		if !s.closed {
			s.resources.mu.Lock()
			s.resources.used -= reserved
			s.resources.mu.Unlock()
			s.used -= reserved
		}
		s.mu.Unlock()
		return nil, err
	}
	return image, nil
}
