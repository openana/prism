// LLM usage: generated with deepseek-v4-pro and modified manually.
package mirrors

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// ---------- Manager Benchmarks ----------

func BenchmarkManager_All_CacheHit(b *testing.B) {
	mirrorCounts := []int{10, 50, 100}
	for _, mc := range mirrorCounts {
		b.Run(fmt.Sprintf("mirrors=%d", mc), func(b *testing.B) {
			b.StopTimer()
			mirrors := makeMirrors(mc)
			host := &mockHost{
				name:    "h1",
				mirrors: mirrors,
			}

			mgr, cancel := newTestManager(
				[]Host{host},
				60*time.Second, // Long TTL so cache stays fresh.
				5*time.Second,
				nil,
				zerolog.Nop(),
			)
			defer cancel()

			// Warm cache: first call triggers fetch.
			it, _ := mgr.All()
			_ = slices.Collect(it)

			b.StartTimer()
			for range b.N {
				it, _ := mgr.All()
				for range it {
				}
			}
		})
	}
}

func BenchmarkManager_All_CacheFresh(b *testing.B) {
	b.StopTimer()
	mirrors := makeMirrors(50)
	host := &mockHost{
		name:    "h1",
		mirrors: mirrors,
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		60*time.Second,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// Warm cache.
	it, _ := mgr.All()
	_ = slices.Collect(it)

	if host.calls.Load() != 1 {
		b.Fatalf("expected 1 call during warmup, got %d", host.calls.Load())
	}

	b.StartTimer()
	for range b.N {
		it, _ := mgr.All()
		for range it {
		}
	}
	b.StopTimer()

	// Cache was fresh — no additional fetch should have occurred.
	if host.calls.Load() != 1 {
		b.Errorf("fetcher called %d times, want 1 (cache should prevent extra calls)",
			host.calls.Load())
	}
}

func BenchmarkManager_All_WithBaseMirrors(b *testing.B) {
	mirrorCounts := []int{10, 50, 100}
	for _, mc := range mirrorCounts {
		b.Run(fmt.Sprintf("mirrors=%d", mc), func(b *testing.B) {
			b.StopTimer()
			mirrors := makeMirrors(mc)
			host := &mockHost{
				name:    "h1",
				mirrors: mirrors,
			}

			// Base mirrors inject Metadata into matching mirrors.
			baseMirrors := make(map[string]Mirror, mc)
			for _, m := range mirrors {
				baseMirrors[m.Name] = Mirror{
					Name: m.Name,
					Metadata: &Metadata{
						Desc: m.Name + " description",
						URL:  "https://" + m.Name + ".org",
					},
				}
			}

			mgr, cancel := newTestManager(
				[]Host{host},
				60*time.Second,
				5*time.Second,
				baseMirrors,
				zerolog.Nop(),
			)
			defer cancel()

			// Warm cache.
			it, _ := mgr.All()
			_ = slices.Collect(it)

			b.StartTimer()
			for range b.N {
				it, _ := mgr.All()
				for range it {
				}
			}
		})
	}
}

func BenchmarkManager_All_ConcurrentReaders(b *testing.B) {
	b.StopTimer()
	mirrors := makeMirrors(100)
	host := &mockHost{
		name:    "h1",
		mirrors: mirrors,
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		60*time.Second,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// Warm cache.
	it, _ := mgr.All()
	_ = slices.Collect(it)

	b.StartTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			it, _ := mgr.All()
			for range it {
			}
		}
	})
}

func BenchmarkManager_All_Fetch(b *testing.B) {
	mirrorCounts := []int{10, 50, 100}
	for _, mc := range mirrorCounts {
		b.Run(fmt.Sprintf("mirrors=%d", mc), func(b *testing.B) {
			b.StopTimer()
			mirrors := makeMirrors(mc)
			host := &mockHost{
				name:    "h1",
				mirrors: mirrors,
			}

			mgr, cancel := newTestManager(
				[]Host{host},
				0, // TTL=0: always stale → forces fetch every call.
				5*time.Second,
				nil,
				zerolog.Nop(),
			)
			defer cancel()

			b.StartTimer()
			for range b.N {
				it, _ := mgr.All()
				_ = slices.Collect(it)
			}
		})
	}
}

// ---------- Helpers ----------

// makeMirrors creates deterministic mirrors for benchmarking.
func makeMirrors(n int) []Mirror {
	mirrors := make([]Mirror, n)
	names := []string{
		"alpine", "archlinux", "centos", "debian", "fedora", "gentoo",
		"kali", "manjaro", "opensuse", "ubuntu",
	}
	for i := range n {
		nameIdx := i % len(names)
		name := names[nameIdx]
		if i >= len(names) {
			name = fmt.Sprintf("%s-%d", name, i/len(names))
		}
		mirrors[i] = Mirror{
			Name: name,
			Sync: &Sync{
				Status:       SyncStatus(i % 7),
				Size:         int64(i * 1024 * 1024),
				LastUpdate:   1652727842 + int64(i),
				LastStarted:  1652727800 + int64(i),
				LastEnded:    1652727900 + int64(i),
				NextSchedule: 1652814242 + int64(i),
			},
		}
	}
	return mirrors
}
