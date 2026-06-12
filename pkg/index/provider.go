package index

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/rs/zerolog"
	"github.com/valyala/bytebufferpool"
)

type CachedProviderConfig interface {
	Fetchers() map[string]FetcherConfig
	TTL() time.Duration
	MaxBytes() int
}

type CachedProvider struct {
	cfg struct {
		ttl time.Duration
	}

	cache *fastcache.Cache

	// Host -> Fetcher
	fetchers map[string]Fetcher

	logger zerolog.Logger
}

func NewCachedProvider(cfg CachedProviderConfig, logger zerolog.Logger) (*CachedProvider, error) {
	p := &CachedProvider{}

	// Build fetchers
	p.fetchers = make(map[string]Fetcher)
	for host, fcfg := range cfg.Fetchers() {
		f, err := BuildFetcher(fcfg, logger)
		if err != nil {
			return nil, fmt.Errorf("NewCachedProvider: %w", err)
		}
		p.fetchers[host] = f
	}

	p.cfg.ttl = cfg.TTL()

	p.cache = fastcache.New(cfg.MaxBytes())

	p.logger = logger.With().Str("module", "index.CachedProvider").Logger()

	return p, nil
}

func (p *CachedProvider) AllOrErr(ctx context.Context, host string, path []byte) (iter.Seq[Entry], error) {
	// Query cache
	buf := bytebufferpool.Get()
	key := bytebufferpool.Get()

	key.WriteString(host)
	key.WriteByte(':')
	key.Write(path)

	var ok bool
	buf.B, ok = p.cache.HasGet(buf.B, key.B)
	if ok {
		b := buf.B

		// Consume ExpiresAt.
		if len(b) >= 8 && time.Unix(int64(binary.NativeEndian.Uint64(b[0:8])), 0).After(time.Now()) {
			b = b[8:]
			bytebufferpool.Put(key)
			return func(yield func(Entry) bool) {
				defer bytebufferpool.Put(buf)
				var e Entry
				var err error

				for {
					b, err = e.ConsumeFrom(b)
					if err != nil {
						return
					}

					if !yield(e) {
						return
					}
				}
			}, nil
		} else {
			// Payload is invalid
			p.cache.Del(key.B)
		}
	}

	// Query upstream
	buf.Reset()

	// Append ExpiresAt.
	buf.B = binary.NativeEndian.AppendUint64(buf.B, uint64(time.Now().Add(p.cfg.ttl).Unix()))

	f, ok := p.fetchers[host]
	if !ok {
		bytebufferpool.Put(buf)
		bytebufferpool.Put(key)
		return nil, fmt.Errorf("CachedProvider: nonexistent host %q", host)
	}

	it, err := f.AllOrErr(ctx, path)
	if err != nil {
		bytebufferpool.Put(buf)
		bytebufferpool.Put(key)
		return nil, err
	}

	return func(yield func(Entry) bool) {
		defer bytebufferpool.Put(buf)
		defer bytebufferpool.Put(key)
		stopYield := false
		for e := range it {
			buf.B = e.AppendTo(buf.B)

			if !stopYield && !yield(e) {
				stopYield = true
			}
		}

		p.cache.Set(key.B, buf.B)
	}, nil
}

func (e *Entry) ConsumeFrom(b []byte) ([]byte, error) {
	if len(b) < 8+8+1+2 {
		return nil, errors.New("ConsumeFrom: payload too short")
	}

	e.Size = int64(binary.NativeEndian.Uint64(b[0:8]))
	e.Mtime = int64(binary.NativeEndian.Uint64(b[8 : 8+8]))
	e.Type = EntryType(b[8+8])
	l := int(binary.NativeEndian.Uint16(b[8+8+1:8+8+1+2])) + 8 + 8 + 1 + 2
	if len(b) < l {
		return nil, errors.New("ConsumeFrom: payload too short")
	}

	e.Name = string(b[8+8+1+2 : l])

	return b[l:], nil
}

func (e *Entry) AppendTo(b []byte) []byte {
	b = binary.NativeEndian.AppendUint64(b, uint64(e.Size))
	b = binary.NativeEndian.AppendUint64(b, uint64(e.Mtime))
	b = append(b, byte(e.Type))
	b = binary.NativeEndian.AppendUint16(b, uint16(len(e.Name)))
	b = append(b, e.Name...)
	return b
}
