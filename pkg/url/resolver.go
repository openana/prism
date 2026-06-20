package url

import (
	"sync"
	"sync/atomic"

	"github.com/openana/prism/pkg/utils/trie"
	"github.com/rs/zerolog"
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

	logger zerolog.Logger
}

type Record struct {
	// "node1"
	Host string
	// "mirrors.example.com"
	FQDN string
	// "mirrors4.example.com"
	FQDNv4 string
	// "mirrors6.example.com"
	FQDNv6 string
	// "/mirror/ubuntu/"
	Prefix string
}

func NewTrieResolver(cfg TrieResolverConfig, logger zerolog.Logger) *TrieResolver {
	rt := &TrieResolver{}

	rt.truth.routes = cfg.Records()
	rt.Commit()

	rt.logger = logger.With().Str("module", "url.TrieResolver").Logger()

	return rt
}

func (rt *TrieResolver) DelRecord(prefix string) (found bool) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	if _, ok := rt.truth.routes[prefix]; ok {
		delete(rt.truth.routes, prefix)
		rt.logger.Debug().Str("prefix", prefix).Msg("record deleted")
		return true
	} else {
		rt.logger.Debug().Str("prefix", prefix).Msg("record not found for deletion")
		return false
	}
}

func (rt *TrieResolver) HasRecord(prefix string) (found bool) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	_, found = rt.truth.routes[prefix]
	rt.logger.Debug().Str("prefix", prefix).Bool("found", found).Msg("has record")
	return
}

func (rt *TrieResolver) SetRecord(prefix string, r Record) {
	rt.truth.Lock()
	defer rt.truth.Unlock()

	rt.logger.Debug().Str("prefix", prefix).Str("host", r.Host).Str("fqdn", r.FQDN).Msg("record set")
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

	rt.logger.Debug().Int("records", len(rt.truth.routes)).Msg("trie committed")
	rt.trie.Store(&trie)
}

func (rt *TrieResolver) Append(path []byte, dst []byte) (result []byte, r Record, ok bool) {
	var l int
	r, l, ok = rt.trie.Load().LongestPrefixMatchWithLen(path)
	if !ok {
		rt.logger.Debug().Bytes("path", path).Msg("path not resolved")
		return
	}

	result = append(dst, r.Prefix...)
	result = append(result, path[l:]...)

	rt.logger.Debug().Bytes("path", path).Bytes("result", result).Str("host", r.Host).Str("fqdn", r.FQDN).Str("fqdn_v4", r.FQDNv4).Str("fqdn_v6", r.FQDNv6).Msg("path resolved")

	return
}
