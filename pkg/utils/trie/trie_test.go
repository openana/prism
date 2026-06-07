/*
LLM usage: this test file was generated with claude-sonnet-4-6 with manual modifications.
*/
package trie_test

import (
	"testing"

	"github.com/openana/prism/pkg/utils/trie"
)

// ---------- PrefixTrie ----------

func TestPrefixTrie_PrecisePrefixMatch(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[int]()
	tb.Add("foo", 1)
	tb.Add("foobar", 2)
	tb.Add("baz", 3)
	tr := tb.Build()

	cases := []struct {
		in   []byte
		want int
		ok   bool
	}{
		{[]byte("foo"), 1, true},
		{[]byte("foobar"), 2, true},
		{[]byte("baz"), 3, true},
		{[]byte("fo"), 0, false},
		{[]byte("fooba"), 0, false},
		{[]byte("foobarx"), 0, false},
		{[]byte(""), 0, false},
	}
	for _, c := range cases {
		got, ok := tr.PrecisePrefixMatch(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("PrecisePrefixMatch(%s) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPrefixTrie_LongestPrefixMatch(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[string]()
	tb.Add("", "root")
	tb.Add("f", "f")
	tb.Add("fo", "fo")
	tb.Add("foo", "foo")
	tb.Add("foobar", "foobar")
	tr := tb.Build()

	cases := []struct {
		in   []byte
		want string
		ok   bool
	}{
		{[]byte("foobar"), "foobar", true},
		{[]byte("foobarbaz"), "foobar", true}, // longest is "foobar"
		{[]byte("foo"), "foo", true},
		{[]byte("fo"), "fo", true},
		{[]byte("f"), "f", true},
		{[]byte(""), "root", true},
		{[]byte("xyz"), "root", true}, // empty string matches
	}
	for _, c := range cases {
		got, ok := tr.LongestPrefixMatch(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("LongestPrefixMatch(%s) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPrefixTrie_LongestPrefixMatch_NoMatch(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[int]()
	tb.Add("foo", 1)
	tr := tb.Build()

	_, ok := tr.LongestPrefixMatch([]byte("bar"))
	if ok {
		t.Error("expected no match for 'bar'")
	}
}

func TestPrefixTrie_LongestPrefixMatchWithLen(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[string]()
	tb.Add("", "root")
	tb.Add("f", "f")
	tb.Add("fo", "fo")
	tb.Add("foo", "foo")
	tb.Add("foobar", "foobar")
	tr := tb.Build()

	cases := []struct {
		in     []byte
		wantV  string
		wantL  int
		wantOK bool
	}{
		{[]byte("foobar"), "foobar", 6, true},
		{[]byte("foobarbaz"), "foobar", 6, true},
		{[]byte("foo"), "foo", 3, true},
		{[]byte("fo"), "fo", 2, true},
		{[]byte("f"), "f", 1, true},
		{[]byte(""), "root", 0, true},
		{[]byte("xyz"), "root", 0, true},
	}
	for _, c := range cases {
		gotV, gotL, ok := tr.LongestPrefixMatchWithLen(c.in)
		if ok != c.wantOK || gotV != c.wantV || gotL != c.wantL {
			t.Errorf("LongestPrefixMatchWithLen(%s) = (%v,%v,%v), want (%v,%v,%v)",
				c.in, gotV, gotL, ok, c.wantV, c.wantL, c.wantOK)
		}
	}
}

func TestPrefixTrie_LongestPrefixMatchWithLen_NoMatch(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[int]()
	tb.Add("foo", 1)
	tr := tb.Build()

	_, l, ok := tr.LongestPrefixMatchWithLen([]byte("bar"))
	if ok {
		t.Error("expected no match for 'bar'")
	}
	if l != 0 {
		t.Errorf("expected length 0 on no match, got %d", l)
	}
}

func TestPrefixTrie_LongestPrefixMatchWithLen_EmptyTrie(t *testing.T) {
	tr := trie.NewPrefixTrie[int]()
	_, l, ok := tr.LongestPrefixMatchWithLen([]byte("anything"))
	if ok {
		t.Error("empty trie should not match")
	}
	if l != 0 {
		t.Errorf("expected length 0 on empty trie, got %d", l)
	}
}

func TestPrefixTrie_OverwriteValue(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[int]()
	tb.Add("key", 1)
	tb.Add("key", 99)
	tr := tb.Build()

	got, ok := tr.PrecisePrefixMatch([]byte("key"))
	if !ok || got != 99 {
		t.Errorf("expected 99, got %v %v", got, ok)
	}
}

func TestPrefixTrie_Empty(t *testing.T) {
	tr := trie.NewPrefixTrie[int]()
	if _, ok := tr.PrecisePrefixMatch([]byte("anything")); ok {
		t.Error("empty trie should not match")
	}
	if _, ok := tr.LongestPrefixMatch([]byte("anything")); ok {
		t.Error("empty trie should not match")
	}
}

func TestPrefixTrie_EmptyKey(t *testing.T) {
	tb := trie.NewPrefixTrieBuilder[string]()
	tb.Add("", "empty")
	tr := tb.Build()

	got, ok := tr.PrecisePrefixMatch([]byte(""))
	if !ok || got != "empty" {
		t.Errorf("expected 'empty', got %v %v", got, ok)
	}
	got, ok = tr.LongestPrefixMatch([]byte("anything"))
	if !ok || got != "empty" {
		t.Errorf("empty prefix should match any string, got %v %v", got, ok)
	}
}

// ---------- SuffixTrie ----------

func TestSuffixTrie_PreciseSuffixMatch(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[int]()
	tb.Add(".com", 1)
	tb.Add(".org", 2)
	tb.Add("example.com", 3)
	tr := tb.Build()

	cases := []struct {
		in   []byte
		want int
		ok   bool
	}{
		{[]byte(".com"), 1, true},
		{[]byte(".org"), 2, true},
		{[]byte("example.com"), 3, true},
		{[]byte("com"), 0, false},
		{[]byte("example.org"), 0, false},
		{[]byte(""), 0, false},
	}
	for _, c := range cases {
		got, ok := tr.PreciseSuffixMatch(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("SuffixTrie.PreciseSuffixMatch(%s) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSuffixTrie_LongestSuffixMatch(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[string]()
	tb.Add("", "root")
	tb.Add("m", "m")
	tb.Add("om", "om")
	tb.Add(".com", ".com")
	tb.Add("e.com", "e.com")
	tr := tb.Build()

	cases := []struct {
		in   []byte
		want string
		ok   bool
	}{
		{[]byte("example.com"), "e.com", true},
		{[]byte("foo.com"), ".com", true},
		{[]byte("foo.com.au"), "root", true}, // no registered suffix matches; falls back to empty-string entry
		{[]byte("anything"), "root", true},
		{[]byte(""), "root", true},
	}
	for _, c := range cases {
		got, ok := tr.LongestSuffixMatch(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("LongestSuffixMatch(%s) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSuffixTrie_LongestSuffixMatch_NoMatch(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[int]()
	tb.Add(".com", 1)
	tr := tb.Build()

	_, ok := tr.LongestSuffixMatch([]byte(".org"))
	if ok {
		t.Error("expected no match")
	}
}

func TestSuffixTrie_LongestSuffixMatchWithLen(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[string]()
	tb.Add("", "root")
	tb.Add("m", "m")
	tb.Add("om", "om")
	tb.Add(".com", ".com")
	tb.Add("e.com", "e.com")
	tr := tb.Build()

	cases := []struct {
		in     []byte
		wantV  string
		wantL  int
		wantOK bool
	}{
		{[]byte("example.com"), "e.com", 5, true},
		{[]byte("foo.com"), ".com", 4, true},
		{[]byte("foo.com.au"), "root", 0, true},
		{[]byte("anything"), "root", 0, true},
		{[]byte(""), "root", 0, true},
	}
	for _, c := range cases {
		gotV, gotL, ok := tr.LongestSuffixMatchWithLen(c.in)
		if ok != c.wantOK || gotV != c.wantV || gotL != c.wantL {
			t.Errorf("LongestSuffixMatchWithLen(%s) = (%v,%v,%v), want (%v,%v,%v)",
				c.in, gotV, gotL, ok, c.wantV, c.wantL, c.wantOK)
		}
	}
}

func TestSuffixTrie_LongestSuffixMatchWithLen_NoMatch(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[int]()
	tb.Add(".com", 1)
	tr := tb.Build()

	_, l, ok := tr.LongestSuffixMatchWithLen([]byte(".org"))
	if ok {
		t.Error("expected no match")
	}
	if l != 0 {
		t.Errorf("expected length 0 on no match, got %d", l)
	}
}

func TestSuffixTrie_LongestSuffixMatchWithLen_EmptyTrie(t *testing.T) {
	tr := trie.NewSuffixTrie[int]()
	_, l, ok := tr.LongestSuffixMatchWithLen([]byte("anything"))
	if ok {
		t.Error("empty trie should not match")
	}
	if l != 0 {
		t.Errorf("expected length 0 on empty trie, got %d", l)
	}
}

func TestSuffixTrie_OverwriteValue(t *testing.T) {
	tb := trie.NewSuffixTrieBuilder[int]()
	tb.Add(".com", 1)
	tb.Add(".com", 42)
	tr := tb.Build()

	got, ok := tr.PreciseSuffixMatch([]byte(".com"))
	if !ok || got != 42 {
		t.Errorf("expected 42, got %v %v", got, ok)
	}
}

func TestSuffixTrie_Empty(t *testing.T) {
	tr := trie.NewSuffixTrie[int]()
	if _, ok := tr.PreciseSuffixMatch([]byte("anything")); ok {
		t.Error("empty trie should not match")
	}
	if _, ok := tr.LongestSuffixMatch([]byte("anything")); ok {
		t.Error("empty trie should not match")
	}
}

// ---------- Benchmarks ----------

func BenchmarkPrefixTrie_LongestPrefixMatch(b *testing.B) {
	tb := trie.NewPrefixTrieBuilder[int]()
	tb.Add("", 0)
	tb.Add("http", 1)
	tb.Add("https", 2)
	tb.Add("https://example", 3)
	tb.Add("https://example.com", 4)
	tr := tb.Build()
	input := []byte("https://example.com/path/to/resource")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.LongestPrefixMatch(input)
	}
}

func BenchmarkSuffixTrie_LongestSuffixMatch(b *testing.B) {
	tb := trie.NewSuffixTrieBuilder[int]()
	tb.Add("", 0)
	tb.Add("m", 1)
	tb.Add("om", 2)
	tb.Add(".com", 3)
	tb.Add("e.com", 4)
	tb.Add("example.com", 5)
	tr := tb.Build()
	input := []byte("sub.example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.LongestSuffixMatch(input)
	}
}
