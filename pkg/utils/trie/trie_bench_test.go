// LLM usage: generated with deepseek-v4-pro and modified manually.
package trie_test

import (
	"fmt"
	"testing"

	"github.com/openana/prism/pkg/utils/trie"
)

// ---------- PrefixTrie Benchmarks ----------

func BenchmarkPrefixTrie_PrecisePrefixMatch(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewPrefixTrieBuilder[int]()
			for i := range n {
				tb.Add(fmt.Sprintf("/mirror/distro%d/", i), i)
			}
			tr := tb.Build()

			// Always match the last prefix (worst case: longest traversal).
			path := []byte(fmt.Sprintf("/mirror/distro%d/", n-1))

			b.ResetTimer()
			for range b.N {
				_, _ = tr.PrecisePrefixMatch(path)
			}
		})
	}
}

func BenchmarkPrefixTrie_LongestPrefixMatch_Scaled(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewPrefixTrieBuilder[int]()
			for i := range n {
				tb.Add(fmt.Sprintf("/mirror/distro%d/", i), i)
			}
			tr := tb.Build()

			// Path that extends beyond the last prefix — forces full traversal.
			path := []byte(fmt.Sprintf("/mirror/distro%d/sub/deep/path", n-1))

			b.ResetTimer()
			for range b.N {
				_, _, _ = tr.LongestPrefixMatchWithLen(path)
			}
		})
	}
}

func BenchmarkPrefixTrie_LongestPrefixMatch_Miss(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewPrefixTrieBuilder[int]()
			for i := range n {
				tb.Add(fmt.Sprintf("/mirror/distro%d/", i), i)
			}
			tr := tb.Build()

			// Path with first byte that doesn't match any branch — early exit.
			path := []byte("/zzz/nomatch")

			b.ResetTimer()
			for range b.N {
				_, _, _ = tr.LongestPrefixMatchWithLen(path)
			}
		})
	}
}

func BenchmarkPrefixTrie_LongestPrefixMatch_RootMatch(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewPrefixTrieBuilder[int]()
			tb.Add("", 0) // root prefix
			for i := range n {
				tb.Add(fmt.Sprintf("/mirror/distro%d/", i), i+1)
			}
			tr := tb.Build()

			// Path that only matches the root ("") — hits root, returns early.
			path := []byte("/nomatch/at/all")

			b.ResetTimer()
			for range b.N {
				_, _, _ = tr.LongestPrefixMatchWithLen(path)
			}
		})
	}
}

func BenchmarkPrefixTrie_ConcurrentReads(b *testing.B) {
	tb := trie.NewPrefixTrieBuilder[int]()
	for i := range 100 {
		tb.Add(fmt.Sprintf("/mirror/distro%d/", i), i)
	}
	tr := tb.Build()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = tr.LongestPrefixMatchWithLen([]byte("/mirror/distro99/sub/path"))
		}
	})
}

func BenchmarkPrefixTrie_Build(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			prefixes := make([]string, n)
			for i := range n {
				prefixes[i] = fmt.Sprintf("/mirror/distro%d/", i)
			}

			b.ResetTimer()
			for range b.N {
				tb := trie.NewPrefixTrieBuilder[int]()
				for i, p := range prefixes {
					tb.Add(p, i)
				}
				_ = tb.Build()
			}
		})
	}
}

// ---------- SuffixTrie Benchmarks ----------

func BenchmarkSuffixTrie_PreciseSuffixMatch(b *testing.B) {
	sizes := []int{10, 50, 100}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewSuffixTrieBuilder[int]()
			for i := range n {
				tb.Add(fmt.Sprintf(".mirror%d.example.com", i), i)
			}
			tr := tb.Build()

			// Match the last suffix.
			path := []byte(fmt.Sprintf("cdn.mirror%d.example.com", n-1))

			b.ResetTimer()
			for range b.N {
				_, _ = tr.PreciseSuffixMatch(path)
			}
		})
	}
}

func BenchmarkSuffixTrie_LongestSuffixMatch_Scaled(b *testing.B) {
	sizes := []int{10, 50, 100}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			tb := trie.NewSuffixTrieBuilder[int]()
			for i := range n {
				tb.Add(fmt.Sprintf(".mirror%d.example.com", i), i)
			}
			tr := tb.Build()

			path := []byte(fmt.Sprintf("cdn.mirror%d.example.com", n-1))

			b.ResetTimer()
			for range b.N {
				_, _, _ = tr.LongestSuffixMatchWithLen(path)
			}
		})
	}
}
