package imagecache

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func event[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("cache work stalled")
		var zero T
		return zero
	}
}

func fixture(t *testing.T, resources *Resources, config Config, loader Loader[*int]) *Store[*int] {
	t.Helper()
	s := New(context.Background(), "/tmp/doc.md", resources, config, loader)
	t.Cleanup(s.Close)
	return s
}

func countedLoader(charge int64) Loader[*int] {
	return func(ctx context.Context, _, _ string, reserve func(int64) error) (*int, error) {
		if err := reserve(charge); err != nil {
			return nil, err
		}
		return new(int), nil
	}
}

func TestDeduplicationAndKeys(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s := fixture(t, NewResources(1024, 2), DefaultConfig(), func(ctx context.Context, dir, key string, reserve func(int64) error) (*int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		if dir != "/tmp" || key != "https://example.com/pic?q=1" {
			t.Errorf("loader inputs: %s %s", dir, key)
		}
		<-release
		return countedLoader(24)(ctx, dir, key, reserve)
	})
	type result struct {
		image *int
		err   error
	}
	results := make(chan result, 12)
	for range 12 {
		go func() { im, err := s.Load("https://EXAMPLE.com/pic?q=1#ignored"); results <- result{im, err} }()
	}
	event(t, started)
	once.Do(func() { close(release) })
	var first *int
	for range 12 {
		r := event(t, results)
		if first == nil {
			first = r.image
		}
		if r.err != nil || r.image == nil || r.image != first {
			t.Fatal("deduplication failed", r)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate loader calls", calls.Load())
	}
	if n, used := s.Stats(); n != 1 || used != 24 {
		t.Fatal(n, used)
	}
	for _, tc := range []struct{ source, want string }{
		{"./one.png", filepath.Clean("/tmp/one.png")},
		{"file:/tmp/one.png", "/tmp/one.png"},
		{"https://example.com/pic?q=2#fragment", "https://example.com/pic?q=2"},
	} {
		if got, err := s.Key(tc.source); err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
	if _, err := s.Load("data:image/png;base64,AAAA"); err == nil {
		t.Fatal("unsupported source admitted")
	}
}

func TestBudgetsAndClose(t *testing.T) {
	r := NewResources(16, 2)
	config := Config{ByteLimit: 8, MaxEntries: 2}
	first, second, third := fixture(t, r, config, countedLoader(8)), fixture(t, r, config, countedLoader(8)), fixture(t, r, config, countedLoader(8))
	a, err := first.Load("one.png")
	if err != nil {
		t.Fatal(err)
	}
	if alias, err := first.Load("./one.png"); err != nil || alias != a {
		t.Fatal("local alias not shared")
	}
	if _, err := first.Load("two.png"); !errors.Is(err, ErrBudget) {
		t.Fatal("document budget ignored", err)
	}
	if _, err := second.Load("one.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := third.Load("one.png"); !errors.Is(err, ErrBudget) {
		t.Fatal("application budget ignored", err)
	}
	first.Close()
	first.Close()
	if r.Used() != 8 {
		t.Fatal("close accounting", r.Used())
	}
	if _, err := third.Load("two.png"); err != nil {
		t.Fatal("released budget unusable", err)
	}
	if _, err := third.Load("three.png"); !errors.Is(err, ErrBudget) {
		t.Fatal("entry limit ignored", err)
	}
	if _, err := first.Load("one.png"); !errors.Is(err, context.Canceled) {
		t.Fatal("closed store accepted work", err)
	}
	second.Close()
	third.Close()
	if r.Used() != 0 {
		t.Fatal("leaked budget", r.Used())
	}
}

func TestFailureReleasesReservations(t *testing.T) {
	r := NewResources(1024, 1)
	failure := errors.New("decode failed")
	var calls int
	s := fixture(t, r, DefaultConfig(), func(ctx context.Context, _, _ string, reserve func(int64) error) (*int, error) {
		calls++
		if err := reserve(8); err != nil {
			return nil, err
		}
		if err := reserve(4); err != nil {
			return nil, err
		}
		return nil, failure
	})
	for range 2 {
		if _, err := s.Load("bad.png"); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	if n, used := s.Stats(); n != 1 || used != 0 || r.Used() != 0 || calls != 1 {
		t.Fatal("failure leaked or retried", n, used, r.Used(), calls)
	}
}

func TestConcurrencyCancellationAndWaiters(t *testing.T) {
	r := NewResources(1024, 1)
	started := make(chan string, 3)
	loader := func(ctx context.Context, _, key string, _ func(int64) error) (*int, error) {
		started <- key
		<-ctx.Done()
		return nil, ctx.Err()
	}
	first, second := fixture(t, r, DefaultConfig(), loader), fixture(t, r, DefaultConfig(), loader)
	done := make(chan error, 3)
	go func() { _, err := first.Load("one.png"); done <- err }()
	event(t, started)
	go func() { _, err := first.Load("one.png"); done <- err }()
	go func() { _, err := second.Load("two.png"); done <- err }()
	select {
	case <-started:
		t.Fatal("global concurrency exceeded")
	case <-time.After(50 * time.Millisecond):
	}
	second.Close()
	first.Close()
	for range 3 {
		if err := event(t, done); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if len(r.slots) != 0 || r.Used() != 0 {
		t.Fatal("cancelled work retained resources")
	}
}

func TestViewportEvictionAndRetry(t *testing.T) {
	r := NewResources(8, 2)
	s := fixture(t, r, Config{ByteLimit: 8, MaxEntries: 128}, countedLoader(8))
	one, _ := s.Key("one.png")
	two, _ := s.Key("two.png")
	for _, key := range []string{one, two, one} {
		s.UpdateViewport(map[string]float32{key: 0})
		if _, err := s.Load(key); err != nil {
			t.Fatal("offscreen entry blocked visible load", err)
		}
		if n, used := s.Stats(); n != 1 || used != 8 || r.Used() != 8 {
			t.Fatal("eviction accounting", n, used, r.Used())
		}
	}
	if _, ok := s.Peek(two); ok {
		t.Fatal("offscreen value retained")
	}
}

func TestRetryCooldownAndScrollCancellation(t *testing.T) {
	var calls int
	s := fixture(t, NewResources(1024, 2), DefaultConfig(), func(ctx context.Context, dir, key string, reserve func(int64) error) (*int, error) {
		calls++
		if calls == 1 {
			return nil, ErrTemporary
		}
		return countedLoader(8)(ctx, dir, key, reserve)
	})
	if _, err := s.Load("one.png"); !errors.Is(err, ErrTemporary) {
		t.Fatal(err)
	}
	s.Load("one.png")
	if calls != 1 {
		t.Fatal("retried before cooldown")
	}
	key, _ := s.Key("one.png")
	s.mu.Lock()
	s.entries[key].retry = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, ready := s.Peek(key); ready {
		t.Fatal("expired failure reported ready")
	}
	if _, err := s.Load(key); err != nil || calls != 2 {
		t.Fatal("temporary failure became permanent", err, calls)
	}

	started := make(chan struct{})
	var attempts atomic.Int32
	cancelStore := fixture(t, NewResources(1024, 2), DefaultConfig(), func(ctx context.Context, dir, key string, reserve func(int64) error) (*int, error) {
		if attempts.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return countedLoader(8)(ctx, dir, key, reserve)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := cancelStore.LoadContext(ctx, "one.png"); done <- err }()
	event(t, started)
	cancel()
	if err := event(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if n, _ := cancelStore.Stats(); n != 0 {
		t.Fatal("cancelled entry retained")
	}
	if _, err := cancelStore.Load("one.png"); err != nil {
		t.Fatal("cancelled entry could not retry", err)
	}
}

func TestSourcePolicyRunsBeforeAdmission(t *testing.T) {
	var loaded bool
	config := DefaultConfig()
	config.AllowSource = func(source string) bool { return source == "allowed.png" }
	s := fixture(t, NewResources(1024, 1), config, func(ctx context.Context, dir, key string, reserve func(int64) error) (*int, error) {
		loaded = true
		return countedLoader(8)(ctx, dir, key, reserve)
	})
	if _, err := s.Load("blocked.png"); err == nil {
		t.Fatal("source policy bypassed")
	}
	if n, _ := s.Stats(); n != 0 || loaded {
		t.Fatal("rejected source admitted or loaded")
	}
	if _, err := s.Load("allowed.png"); err != nil || !loaded {
		t.Fatal("allowed source rejected", err)
	}
}

func TestCloseDiscardsLateResultAndReturnsBudgetOnce(t *testing.T) {
	r := NewResources(1024, 1)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s := fixture(t, r, DefaultConfig(), func(ctx context.Context, dir, key string, reserve func(int64) error) (*int, error) {
		if err := reserve(8); err != nil {
			return nil, err
		}
		close(started)
		<-release // Model an uninterruptible decoder.
		return new(int), nil
	})
	done := make(chan error, 1)
	go func() { _, err := s.Load("one.png"); done <- err }()
	event(t, started)
	s.Close()
	if r.Used() != 0 {
		t.Fatal("close retained budget")
	}
	once.Do(func() { close(release) })
	if err := event(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal("late result published", err)
	}
	if n, used := s.Stats(); n != 0 || used != 0 || r.Used() != 0 {
		t.Fatal("late result changed accounting", n, used, r.Used())
	}
}

func TestWarmEvictionProtectsVisibleAndPendingEntries(t *testing.T) {
	r := NewResources(1024, 2)
	s := fixture(t, r, DefaultConfig(), countedLoader(0))
	for i := range 64 {
		if _, err := s.Load(fmt.Sprintf("%d.png", i)); err != nil {
			t.Fatal(err)
		}
	}
	visible, _ := s.Key("0.png")
	pending, _ := s.Key("pending.png")
	// Model an unfinished loader: it must remain pinned even offscreen.
	s.entries[pending] = &entry[*int]{done: make(chan struct{})}
	s.UpdateViewport(map[string]float32{visible: 0})
	if n, _ := s.Stats(); n != 63 || !s.Contains(visible) || !s.Contains(pending) {
		t.Fatal("warm eviction boundary or pinned entry protection changed", n)
	}
}
