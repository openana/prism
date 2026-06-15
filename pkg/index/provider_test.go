// LLM usage: generated with deepseek-v4-pro and modified manually.
package index

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockFetcher implements Fetcher and tracks calls.
type mockFetcher struct {
	mu        sync.Mutex
	entries   []Entry
	err       error
	callCount atomic.Int32
}

func (m *mockFetcher) AllOrErr(ctx context.Context, path []byte) (iter.Seq[Entry], error) {
	m.callCount.Add(1)
	if m.err != nil {
		return nil, m.err
	}

	// Copy entries to avoid races if caller modifies.
	m.mu.Lock()
	entries := make([]Entry, len(m.entries))
	copy(entries, m.entries)
	m.mu.Unlock()

	return func(yield func(Entry) bool) {
		for _, e := range entries {
			if !yield(e) {
				return
			}
		}
	}, nil
}

func (m *mockFetcher) calls() int32 {
	return m.callCount.Load()
}

// mockCachedProviderConfig implements CachedProviderConfig.
type mockCachedProviderConfig struct {
	ttl      time.Duration
	maxBytes int
	fetchers map[string]FetcherConfig
}

func (m mockCachedProviderConfig) TTL() time.Duration                 { return m.ttl }
func (m mockCachedProviderConfig) MaxBytes() int                      { return m.maxBytes }
func (m mockCachedProviderConfig) Fetchers() map[string]FetcherConfig { return m.fetchers }

// newTestProvider creates a CachedProvider for testing with mock fetchers.
func newTestProvider(ttl time.Duration, maxBytes int, fetchers map[string]Fetcher) *CachedProvider {
	fcfgs := make(map[string]FetcherConfig, len(fetchers))
	for host, f := range fetchers {
		fcfgs[host] = GenericFetcherConfig{F: f}
	}
	p, err := NewCachedProvider(mockCachedProviderConfig{
		ttl:      ttl,
		maxBytes: maxBytes,
		fetchers: fcfgs,
	}, zerolog.Nop())
	if err != nil {
		panic(err)
	}
	return p
}

func TestCachedProvider_AllOrErr_CacheMissThenHit(t *testing.T) {
	mock := &mockFetcher{
		entries: []Entry{
			{Name: "alpine-v3.16", Type: Directory, Mtime: 1652727842},
			{Name: "README.txt", Type: File, Size: 42, Mtime: 1652727842},
		},
	}

	p := newTestProvider(10*time.Second, 1024*1024, map[string]Fetcher{
		"alpine.example.com": mock,
	})

	// First call: cache miss, should call fetcher.
	it, age, err := p.AllOrErr(context.Background(), "alpine.example.com", []byte("/v3.16/"))
	_ = age
	if err != nil {
		t.Fatalf("first AllOrErr() unexpected error: %v", err)
	}

	var entries []Entry
	for e := range it {
		entries = append(entries, e)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name != "alpine-v3.16" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "alpine-v3.16")
	}
	if entries[1].Name != "README.txt" {
		t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "README.txt")
	}
	if mock.calls() != 1 {
		t.Errorf("fetcher called %d times, want 1", mock.calls())
	}

	// Second call: cache hit, should NOT call fetcher.
	it2, age, err := p.AllOrErr(context.Background(), "alpine.example.com", []byte("/v3.16/"))
	_ = age
	if err != nil {
		t.Fatalf("second AllOrErr() unexpected error: %v", err)
	}

	var entries2 []Entry
	for e := range it2 {
		entries2 = append(entries2, e)
	}

	if len(entries2) != 2 {
		t.Fatalf("got %d entries on cache hit, want 2", len(entries2))
	}
	if entries2[0].Name != "alpine-v3.16" {
		t.Errorf("cache hit entries[0].Name = %q, want %q", entries2[0].Name, "alpine-v3.16")
	}
	if mock.calls() != 1 {
		t.Errorf("fetcher called %d times on second call, want still 1 (cache hit)", mock.calls())
	}
}

func TestCachedProvider_AllOrErr_NonexistentHost(t *testing.T) {
	p := newTestProvider(10*time.Second, 1024*1024, map[string]Fetcher{})

	_, _, err := p.AllOrErr(context.Background(), "no.such.host", []byte("/path"))
	if err == nil {
		t.Fatal("expected error for nonexistent host, got nil")
	}
}

func TestCachedProvider_AllOrErr_UpstreamError(t *testing.T) {
	mock := &mockFetcher{
		err: context.DeadlineExceeded,
	}

	p := newTestProvider(10*time.Second, 1024*1024, map[string]Fetcher{
		"err.example.com": mock,
	})

	_, _, err := p.AllOrErr(context.Background(), "err.example.com", []byte("/path"))
	if err == nil {
		t.Fatal("expected error from upstream fetcher, got nil")
	}
	if mock.calls() != 1 {
		t.Errorf("fetcher called %d times, want 1", mock.calls())
	}

	// Second call: upstream error should NOT be cached — fetcher called again.
	_, _, err = p.AllOrErr(context.Background(), "err.example.com", []byte("/path"))
	if err == nil {
		t.Fatal("expected error from upstream fetcher on retry, got nil")
	}
	if mock.calls() != 2 {
		t.Errorf("fetcher called %d times on retry, want 2 (errors not cached)", mock.calls())
	}
}

func TestCachedProvider_AllOrErr_StaleCacheEviction(t *testing.T) {
	mock := &mockFetcher{
		entries: []Entry{
			{Name: "fresh", Type: File, Size: 1, Mtime: 1},
		},
	}

	// Use a very short TTL that will expire before the second call.
	p := newTestProvider(1*time.Nanosecond, 1024*1024, map[string]Fetcher{
		"test.example.com": mock,
	})

	// First call: populate cache.
	it, age, err := p.AllOrErr(context.Background(), "test.example.com", []byte("/v1/"))
	_ = age
	if err != nil {
		t.Fatalf("first AllOrErr() unexpected error: %v", err)
	}
	for range it {
	}
	if mock.calls() != 1 {
		t.Fatalf("fetcher called %d times, want 1", mock.calls())
	}

	// Wait for TTL to expire.
	time.Sleep(10 * time.Millisecond)

	// Second call: cache should be stale, fetcher called again.
	it2, age, err := p.AllOrErr(context.Background(), "test.example.com", []byte("/v1/"))
	_ = age
	if err != nil {
		t.Fatalf("second AllOrErr() unexpected error: %v", err)
	}
	var entries []Entry
	for e := range it2 {
		entries = append(entries, e)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if mock.calls() != 2 {
		t.Errorf("fetcher called %d times, want 2 (stale cache should re-fetch)", mock.calls())
	}
}

func TestCachedProvider_AllOrErr_EmptyPath(t *testing.T) {
	mock := &mockFetcher{
		entries: []Entry{
			{Name: "root-file", Type: File, Size: 99, Mtime: 100},
		},
	}

	p := newTestProvider(10*time.Second, 1024*1024, map[string]Fetcher{
		"x.example.com": mock,
	})

	// Empty path should work.
	it, age, err := p.AllOrErr(context.Background(), "x.example.com", []byte(""))
	_ = age
	if err != nil {
		t.Fatalf("AllOrErr() with empty path unexpected error: %v", err)
	}
	var entries []Entry
	for e := range it {
		entries = append(entries, e)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Name != "root-file" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "root-file")
	}
	if mock.calls() != 1 {
		t.Errorf("fetcher called %d times, want 1", mock.calls())
	}

	// Cache hit with empty path.
	it2, age, err := p.AllOrErr(context.Background(), "x.example.com", []byte(""))
	_ = age
	if err != nil {
		t.Fatalf("second AllOrErr() unexpected error: %v", err)
	}
	for range it2 {
	}
	if mock.calls() != 1 {
		t.Errorf("fetcher called %d times on cache hit, want 1", mock.calls())
	}
}
