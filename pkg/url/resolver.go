package url

import (
	"sync"
	"sync/atomic"

	"github.com/openana/prism/pkg/utils/trie"
)

type Resolver interface {
	Append(path []byte, dst []byte) (result []byte, r Record, ok bool)
}

type TrieResolverConfig interface {
	Records() map[string]Record
}

type TrieResolver struct {
	trie atomic.Pointer[trie.PrefixTrie[Record]]

	truth struct {
		sync.Mutex
		routes map[string]Record
	}
}

type Record struct {
	// "node1"
	Host string
	// "mirrors.example.com"
	FQDN string
	// "/mirror/ubuntu/"
	Prefix string
}

func NewTrieResolver(cfg TrieResolverConfig) *TrieResolver {
	rt := &TrieResolver{}

	rt.truth.routes = cfg.Records()
	rt.Commit()

	return rt
}

func (rt *TrieResolver) DelRecord(prefix string) (found bool) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	if _, ok := rt.truth.routes[prefix]; ok {
		delete(rt.truth.routes, prefix)
		return true
	} else {
		return false
	}
}

func (rt *TrieResolver) HasRecord(prefix string) (found bool) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	_, found = rt.truth.routes[prefix]
	return
}

func (rt *TrieResolver) SetRecord(prefix string, r Record) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	rt.truth.routes[prefix] = r
}

func (rt *TrieResolver) Commit() {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	tb := trie.NewPrefixTrieBuilder[Record]()

	for k, v := range rt.truth.routes {
		tb.Add(k, v)
	}

	trie := tb.Build()
	rt.trie.Store(&trie)
}

func (rt *TrieResolver) Append(path []byte, dst []byte) (result []byte, r Record, ok bool) {
	var l int
	r, l, ok = rt.trie.Load().LongestPrefixMatchWithLen(path)
	if !ok {
		return
	}

	result = append(dst, r.Prefix...)
	result = append(dst, path[l:]...)
	return
}
