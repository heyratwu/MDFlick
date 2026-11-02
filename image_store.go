package main

import (
	"context"
	"errors"
	"mdflick/internal/diagnostics"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"mdflick/internal/document"
)

const (
	documentImageBudget    int64 = 128 << 20
	applicationImageBudget int64 = 256 << 20
	documentImageLimit           = 128
)

var errImageBudget = errors.New("图片缓存已达上限，稍后可重试")

// Account for active document caches. This is an estimate, not a limit on total
// process/GPU memory. Views retain geometry independently when offscreen bitmaps are reclaimed.
type imageResources struct {
	mu          sync.Mutex
	used, limit int64
	slots       chan struct{}
}

type imageEntry struct {
	done  chan struct{}
	image ui.ImageSource
	err   error
	bytes int64
	retry time.Time
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
	wanted      map[string]float32
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
	s.wanted = nil
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
	return s.loadContext(s.ctx, source)
}

// A viewport job may be cancelled without closing the document cache.
func (s *imageStore) loadContext(ctx context.Context, source string) (ui.ImageSource, error) {
	if err := ctx.Err(); err != nil {
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
	if entry := s.entries[key]; entry != nil && !entry.retry.IsZero() && !time.Now().Before(entry.retry) {
		delete(s.entries, key)
	}
	if entry := s.entries[key]; entry != nil {
		s.mu.Unlock()
		select {
		case <-entry.done:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return entry.image, entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.evictLocked(0, false, true)
	if len(s.entries) >= s.maxEntries {
		s.mu.Unlock()
		return nil, errImageBudget
	}
	entry := &imageEntry{done: make(chan struct{})}
	s.entries[key] = entry
	s.mu.Unlock()

	image, err := s.loadOnce(ctx, key)
	s.mu.Lock()
	if s.closed {
		image, err = nil, context.Canceled
	}
	entry.image, entry.err = image, err
	var network net.Error
	if errors.Is(err, errImageBudget) || errors.Is(err, errImageTemporary) || errors.As(err, &network) {
		entry.retry = time.Now().Add(2 * time.Second)
	}
	// A scroll cancellation is retryable when the image returns to the viewport.
	// Delete only our own entry, before waking any duplicate waiter.
	if errors.Is(err, context.Canceled) && s.entries[key] == entry {
		delete(s.entries, key)
	}
	close(entry.done)
	s.mu.Unlock()
	return image, err
}

func (s *imageStore) loadOnce(ctx context.Context, key string) (ui.ImageSource, error) {
	started := time.Now()
	defer func() {
		diagnostics.Record(ctx, "native.image-load-decode", started, 0)
		diagnostics.Memory("native.image-load-decode")
	}()
	if err := acquireImageSlot(ctx, s.slots); err != nil {
		return nil, err
	}
	defer func() { <-s.slots }()
	if err := acquireImageSlot(ctx, s.resources.slots); err != nil {
		return nil, err
	}
	defer func() { <-s.resources.slots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var reserved int64
	reserve := func(bytes int64) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || ctx.Err() != nil {
			return context.Canceled
		}
		s.evictLocked(bytes, false, false)
		s.resources.mu.Lock()
		defer s.resources.mu.Unlock()
		if bytes > s.limit-s.used || bytes > s.resources.limit-s.resources.used {
			return errImageBudget
		}
		s.used += bytes
		s.resources.used += bytes
		reserved += bytes
		if entry := s.entries[key]; entry != nil {
			entry.bytes += bytes
		}
		return nil
	}
	image, err := loadDocumentImage(ctx, s.dir, key, reserve)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		s.mu.Lock()
		if entry := s.entries[key]; entry != nil {
			entry.bytes = 0
		}
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

// peek never admits a URL or starts work. Only dispatched jobs occupy entries.
func (s *imageStore) peek(key string) (ui.ImageSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[key]
	if entry == nil {
		return nil, false
	}
	select {
	case <-entry.done:
		if !entry.retry.IsZero() && !time.Now().Before(entry.retry) {
			return nil, false
		}
		return entry.image, true
	default:
		return nil, false
	}
}

func (s *imageStore) canAdmit(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked(0, false, true)
	return !s.closed && (s.entries[key] != nil || len(s.entries) < s.maxEntries)
}

const imageWarmBudget int64 = 16 << 20

// Called with the store lock; resource accounting always follows that lock.
// Active and visible entries are pinned. No cross-store lock is acquired.
func (s *imageStore) evictLocked(incoming int64, warm, admission bool) {
	if s.wanted == nil {
		return
	} // direct loader callers have no viewport contract
	s.resources.mu.Lock()
	defer s.resources.mu.Unlock()
	for key, entry := range s.entries {
		if _, keep := s.wanted[key]; keep {
			continue
		}
		select {
		case <-entry.done:
		default:
			continue
		}
		if warm && s.used <= imageWarmBudget && len(s.entries) < 64 {
			break
		}
		if !warm && admission && len(s.entries) < s.maxEntries {
			break
		}
		if !warm && !admission && incoming <= s.limit-s.used && incoming <= s.resources.limit-s.resources.used {
			break
		}
		s.used -= entry.bytes
		s.resources.used -= entry.bytes
		delete(s.entries, key)
	}
}
func (s *imageStore) updateViewport(wanted map[string]float32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.used
	s.wanted = wanted
	s.evictLocked(0, true, false)
	return s.used != before
}
