// LLM usage: generated with deepseek-v4-pro and modified manually.
package index

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// ---------- CachedProvider Benchmarks ----------

func BenchmarkCachedProvider_CacheHit(b *testing.B) {
	entryCounts := []int{10, 100, 1000}
	for _, ec := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", ec), func(b *testing.B) {
			b.StopTimer()
			entries := makeEntries(ec)
			mock := &mockFetcher{entries: entries}

			p := newTestProviderWithCache(10*time.Second, 64*1024*1024, map[string]Fetcher{
				"host.example.com": mock,
			}, entries, "/test/")

			b.StartTimer()
			for range b.N {
				it, _, err := p.AllOrErr(context.Background(), "host.example.com", []byte("/test/"))
				if err != nil {
					b.Fatalf("cache hit failed: %v", err)
				}
				// Consume all entries.
				for range it {
				}
			}
		})
	}
}

func BenchmarkCachedProvider_EntryDeserialization(b *testing.B) {
	entryCounts := []int{10, 100, 1000}
	for _, ec := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", ec), func(b *testing.B) {
			b.StopTimer()
			entries := makeEntries(ec)

			// Serialize entries directly (matching cache format).
			cached := serializeEntries(entries, 10*time.Second)
			// Skip the 8-byte ExpiresAt header.
			cached = cached[8:]

			b.StartTimer()
			for range b.N {
				buf := cached
				var e Entry
				var err error
				for len(buf) > 0 {
					buf, err = e.ConsumeFrom(buf)
					if err != nil {
						b.Fatalf("deserialization failed: %v", err)
					}
				}
			}
		})
	}
}

func BenchmarkCachedProvider_CacheMiss(b *testing.B) {
	entryCounts := []int{10, 100, 1000}
	for _, ec := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", ec), func(b *testing.B) {
			b.StopTimer()
			entries := makeEntries(ec)
			mock := &mockFetcher{entries: entries}

			p := newTestProvider(10*time.Second, 64*1024*1024, map[string]Fetcher{
				"host.example.com": mock,
			})

			b.StartTimer()
			for range b.N {
				b.StopTimer()
				// Use a unique path each iteration to force a cache miss.
				path := []byte(fmt.Sprintf("/test/miss/%d/", b.N))
				mock.mu.Lock()
				mock.entries = entries // Reset entries (they get copied by mockFetcher).
				mock.mu.Unlock()
				b.StartTimer()

				it, _, err := p.AllOrErr(context.Background(), "host.example.com", path)
				if err != nil {
					b.Fatalf("cache miss failed: %v", err)
				}
				for range it {
				}
			}
		})
	}
}

func BenchmarkCachedProvider_ConcurrentReads(b *testing.B) {
	entryCounts := []int{10, 100, 1000}
	for _, ec := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", ec), func(b *testing.B) {
			b.StopTimer()
			entries := makeEntries(ec)
			mock := &mockFetcher{entries: entries}

			p := newTestProviderWithCache(10*time.Second, 64*1024*1024, map[string]Fetcher{
				"host.example.com": mock,
			}, entries, "/test/")

			b.StartTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					it, _, err := p.AllOrErr(context.Background(), "host.example.com", []byte("/test/"))
					if err != nil {
						b.Errorf("cache hit failed: %v", err)
						return
					}
					for range it {
					}
				}
			})
		})
	}
}

func BenchmarkCachedProvider_MissThenHit(b *testing.B) {
	entryCounts := []int{10, 100, 1000}
	for _, ec := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", ec), func(b *testing.B) {
			entries := makeEntries(ec)
			mock := &mockFetcher{entries: entries}

			p := newTestProvider(10*time.Second, 64*1024*1024, map[string]Fetcher{
				"host.example.com": mock,
			})

			path := []byte("/test/missThenHit/")

			b.ResetTimer()
			for range b.N {
				it, _, err := p.AllOrErr(context.Background(), "host.example.com", path)
				if err != nil {
					b.Fatalf("AllOrErr failed: %v", err)
				}
				for range it {
				}
			}
		})
	}
}

// ---------- Helpers ----------

// makeEntries creates deterministic entries for benchmarking.
func makeEntries(n int) []Entry {
	entries := make([]Entry, n)
	for i := range n {
		entries[i] = Entry{
			Name:  fmt.Sprintf("file-%04d.tar.gz", i),
			Size:  int64(i * 1024),
			Mtime: 1652727842 + int64(i),
			Type:  File,
		}
	}
	return entries
}

// serializeEntries serializes entries with an ExpiresAt header into binary format,
// matching the on-disk cache format used by CachedProvider.
func serializeEntries(entries []Entry, ttl time.Duration) []byte {
	expiresAt := time.Now().Add(ttl).Unix()
	buf := make([]byte, 8)
	buf = binary.NativeEndian.AppendUint64(buf[:0], uint64(expiresAt))
	for i := range entries {
		buf = entries[i].AppendTo(buf)
	}
	return buf
}

// newTestProviderWithCache creates a CachedProvider with a pre-populated fastcache.
// The cache is loaded directly with serialized entry data, bypassing the normal
// fetch path so that cache-hit benchmarks don't count fetch calls.
func newTestProviderWithCache(ttl time.Duration, maxBytes int, fetchers map[string]Fetcher, entries []Entry, path string) *CachedProvider {
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

	// Pre-populate cache directly.
	data := serializeEntries(entries, ttl)
	for host := range fetchers {
		key := host + ":" + path
		p.cache.Set([]byte(key), data)
	}
	return p
}
