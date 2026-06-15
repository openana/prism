// LLM usage: generated with deepseek-v4-pro and modified manually.
package url

import (
	"fmt"
	"io"
	"testing"

	"github.com/rs/zerolog"
)

// ---------- TrieResolver Benchmarks ----------

func BenchmarkTrieResolver_Append_Hit(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			records := make(map[string]Record, n)
			for i := range n {
				prefix := fmt.Sprintf("/mirror/distro%d/", i)
				records[prefix] = Record{
					Host:   fmt.Sprintf("node%d", i),
					FQDN:   fmt.Sprintf("mirrors%d.example.com", i),
					Prefix: prefix,
				}
			}
			rt := helpResolve(records)

			// Match the deepest prefix — worst case traversal.
			path := []byte(fmt.Sprintf("/mirror/distro%d/sub/deep/path", n-1))

			b.ResetTimer()
			for range b.N {
				_, _, _ = rt.Append(path, nil)
			}
		})
	}
}

func BenchmarkTrieResolver_Append_Miss(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			records := make(map[string]Record, n)
			for i := range n {
				prefix := fmt.Sprintf("/mirror/distro%d/", i)
				records[prefix] = Record{
					Host:   fmt.Sprintf("node%d", i),
					FQDN:   fmt.Sprintf("mirrors%d.example.com", i),
					Prefix: prefix,
				}
			}
			rt := helpResolve(records)

			// Path that doesn't match any prefix.
			path := []byte("/zzz/nomatch/here")

			b.ResetTimer()
			for range b.N {
				_, _, _ = rt.Append(path, nil)
			}
		})
	}
}

func BenchmarkTrieResolver_Append_WithDst(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			records := make(map[string]Record, n)
			for i := range n {
				prefix := fmt.Sprintf("/mirror/distro%d/", i)
				records[prefix] = Record{
					Host:   fmt.Sprintf("node%d", i),
					FQDN:   fmt.Sprintf("mirrors%d.example.com", i),
					Prefix: prefix,
				}
			}
			rt := helpResolve(records)

			path := []byte(fmt.Sprintf("/mirror/distro%d/sub/path", n-1))

			b.ResetTimer()
			for range b.N {
				_, _, _ = rt.Append(path, make([]byte, 0, 256))
			}
		})
	}
}

func BenchmarkTrieResolver_Append_Concurrent(b *testing.B) {
	records := make(map[string]Record, 100)
	for i := range 100 {
		prefix := fmt.Sprintf("/mirror/distro%d/", i)
		records[prefix] = Record{
			Host:   fmt.Sprintf("node%d", i),
			FQDN:   fmt.Sprintf("mirrors%d.example.com", i),
			Prefix: prefix,
		}
	}
	rt := helpResolve(records)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := []byte(fmt.Sprintf("/mirror/distro%d/sub/path", i%100))
			_, _, _ = rt.Append(path, nil)
			i++
		}
	})
}

func BenchmarkTrieResolver_Commit(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.StopTimer()
			rt := NewTrieResolver(stubConfig{records: map[string]Record{}}, zerolog.New(io.Discard))
			b.StartTimer()

			for range b.N {
				rt.truth.Lock()
				rt.truth.routes = make(map[string]Record, n)
				for i := range n {
					prefix := fmt.Sprintf("/mirror/distro%d/", i)
					rt.truth.routes[prefix] = Record{
						Host:   fmt.Sprintf("node%d", i),
						FQDN:   fmt.Sprintf("mirrors%d.example.com", i),
						Prefix: prefix,
					}
				}
				rt.truth.Unlock()
				rt.Commit()
			}
		})
	}
}
