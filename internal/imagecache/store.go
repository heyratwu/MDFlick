// Package imagecache owns image admission, deduplication, cancellation and cache budgets.
// Loaders supply decoded values; the cache has no UI or decoder dependency.
package imagecache

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	documentImageBudget int64 = 128 << 20
	DefaultEntryLimit         = 128
)

var ErrBudget = errors.New("图片缓存已达上限，稍后可重试")

// ErrTemporary marks a loader failure that may be retried after the cooldown.
var ErrTemporary = errors.New("图片请求暂时失败")

// Account for active document caches. This is an estimate, not a limit on total
// process/GPU memory. Views retain geometry independently when offscreen bitmaps are reclaimed.
type Resources struct {
	budget *ByteBudget
	slots  chan struct{}
}

type entry[T any] struct {
	done  chan struct{}
	image T
	err   error
	bytes int64
	retry time.Time
}

// Store shares decoded values within one document. Its methods are safe for concurrent use.
type Store[T any] struct {
	ctx         context.Context
	cancel      context.CancelFunc
	dir         string
	resources   *Resources
	slots       chan struct{}
	mu          sync.Mutex
	entries     map[string]*entry[T]
	used, limit int64
	maxEntries  int
	closed      bool
	loader      Loader[T]
	allowSource func(string) bool
	wanted      map[string]float32
}

// Loader reserves retained bytes before allocating the decoded value. A failed
// load returns all reservations. It must honor ctx and not retain reserve after returning.
type Loader[T any] func(ctx context.Context, dir, source string, reserve func(int64) error) (T, error)

// Config sets per-document limits. Use DefaultConfig for native defaults.
type Config struct {
	ByteLimit  int64
	MaxEntries int
	// AllowSource optionally restricts addresses before normalization and admission.
	AllowSource func(string) bool
}

func DefaultConfig() Config {
	return Config{ByteLimit: documentImageBudget, MaxEntries: DefaultEntryLimit}
}

func NewResources(limit int64, concurrency int) *Resources {
	return &Resources{budget: NewByteBudget(limit), slots: make(chan struct{}, concurrency)}
}

// Used reports estimated retained bytes across all stores sharing these resources.
func (r *Resources) Used() int64 { return r.budget.Used() }

// New creates a document cache. Resources must be shared by stores belonging
// to the same application, and loader must be non-nil.
func New[T any](parent context.Context, path string, resources *Resources, config Config, loader Loader[T]) *Store[T] {
	ctx, cancel := context.WithCancel(parent)
	s := &Store[T]{ctx: ctx, cancel: cancel, dir: filepath.Dir(path), resources: resources,
		slots: make(chan struct{}, 2), entries: make(map[string]*entry[T]),
		limit: config.ByteLimit, maxEntries: config.MaxEntries, loader: loader, allowSource: config.AllowSource}
	context.AfterFunc(ctx, s.Close)
	return s
}

// Context ends when the document or store is closed.
func (s *Store[T]) Context() context.Context { return s.ctx }

// Stats returns a consistent snapshot, including pending and failed entries.
func (s *Store[T]) Stats() (entries int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries), s.used
}

// Contains reports admission without starting work or changing retry state.
func (s *Store[T]) Contains(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.entries[key]
	return ok
}

func (s *Store[T]) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.resources.budget.Release(s.used)
	s.used = 0
	clear(s.entries)
	s.wanted = nil
}

// Normalize local paths and URL fragments before deduplicating. Query strings
// remain significant; redirects are handled by the HTTP client.
func (s *Store[T]) Key(source string) (string, error) {
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

func (s *Store[T]) Load(source string) (T, error) {
	return s.LoadContext(s.ctx, source)
}

// LoadContext lets a viewport job be cancelled without closing the cache.
// Callers should derive ctx from Context so document cancellation also stops work.
func (s *Store[T]) LoadContext(ctx context.Context, source string) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if s.allowSource != nil && !s.allowSource(source) {
		return zero, errors.New("不支持的图片地址协议")
	}
	key, err := s.Key(source)
	if err != nil {
		return zero, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return zero, context.Canceled
	}
	if entry := s.entries[key]; entry != nil && !entry.retry.IsZero() && !time.Now().Before(entry.retry) {
		delete(s.entries, key)
	}
	if entry := s.entries[key]; entry != nil {
		s.mu.Unlock()
		select {
		case <-entry.done:
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			return entry.image, entry.err
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	s.evictLocked(0, false, true)
	if len(s.entries) >= s.maxEntries {
		s.mu.Unlock()
		return zero, ErrBudget
	}
	entry := &entry[T]{done: make(chan struct{})}
	s.entries[key] = entry
	s.mu.Unlock()

	image, err := s.loadOnce(ctx, key)
	s.mu.Lock()
	if s.closed {
		image, err = zero, context.Canceled
	}
	entry.image, entry.err = image, err
	var network net.Error
	if errors.Is(err, ErrBudget) || errors.Is(err, ErrTemporary) || errors.As(err, &network) {
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

func (s *Store[T]) loadOnce(ctx context.Context, key string) (T, error) {
	var zero T
	if err := acquireImageSlot(ctx, s.slots); err != nil {
		return zero, err
	}
	defer func() { <-s.slots }()
	if err := acquireImageSlot(ctx, s.resources.slots); err != nil {
		return zero, err
	}
	defer func() { <-s.resources.slots }()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	var reserved int64
	reserve := func(bytes int64) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || ctx.Err() != nil {
			return context.Canceled
		}
		s.evictLocked(bytes, false, false)
		if bytes < 0 || bytes > s.limit-s.used || !s.resources.budget.TryReserve(bytes) {
			return ErrBudget
		}
		s.used += bytes
		reserved += bytes
		if entry := s.entries[key]; entry != nil {
			entry.bytes += bytes
		}
		return nil
	}
	image, err := s.loader(ctx, s.dir, key, reserve)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		s.mu.Lock()
		if entry := s.entries[key]; entry != nil {
			entry.bytes = 0
		}
		if !s.closed {
			s.resources.budget.Release(reserved)
			s.used -= reserved
		}
		s.mu.Unlock()
		return zero, err
	}
	return image, nil
}

// Peek never admits a URL or starts work. Only dispatched jobs occupy entries.
func (s *Store[T]) Peek(key string) (T, bool) {
	var zero T
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[key]
	if entry == nil {
		return zero, false
	}
	select {
	case <-entry.done:
		if !entry.retry.IsZero() && !time.Now().Before(entry.retry) {
			return zero, false
		}
		return entry.image, true
	default:
		return zero, false
	}
}

// CanAdmit reclaims eligible offscreen entries and reports whether a key can be loaded.
func (s *Store[T]) CanAdmit(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked(0, false, true)
	return !s.closed && (s.entries[key] != nil || len(s.entries) < s.maxEntries)
}

const imageWarmBudget int64 = 16 << 20

// Called with the store lock; resource accounting always follows that lock.
// Active and visible entries are pinned. No cross-store lock is acquired.
func (s *Store[T]) evictLocked(incoming int64, warm, admission bool) {
	if s.wanted == nil {
		return
	} // direct loader callers have no viewport contract
	budget := s.resources.budget
	budget.mu.Lock()
	defer budget.mu.Unlock()
	policy := EvictionPolicy{WarmBytes: imageWarmBudget, WarmEntries: 64, ByteLimit: s.limit, EntryLimit: s.maxEntries}
	mode := ReserveBytes
	if warm {
		mode = TrimWarm
	} else if admission {
		mode = AdmitEntry
	}
	for key, entry := range s.entries {
		if _, keep := s.wanted[key]; keep {
			continue
		}
		select {
		case <-entry.done:
		default:
			continue
		}
		if !policy.NeedsEviction(mode, s.used, len(s.entries), incoming, budget.limit-budget.used) {
			break
		}
		s.used -= entry.bytes
		budget.releaseLocked(entry.bytes)
		delete(s.entries, key)
	}
}

// UpdateViewport replaces the wanted keys and reclaims excess offscreen values.
// Ownership of wanted transfers to the store; the caller must not mutate it.
// The return value indicates whether bytes were reclaimed.
func (s *Store[T]) UpdateViewport(wanted map[string]float32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.used
	s.wanted = wanted
	s.evictLocked(0, true, false)
	return s.used != before
}
