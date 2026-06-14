package mirrors

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"
)

const (
	initialBackoff = 5 * time.Second
	maxBackoff     = time.Minute
)

type ManagerConfig interface {
	Hosts() []HostConfig
	CacheTTL() time.Duration
	FetchTimeout() time.Duration
	BaseMirrors() map[string]Mirror
	MirrorzSite() *Site
	MirrorzInfo() []Info
}

type Manager struct {
	cfg struct {
		baseMirrors    map[string]Mirror
		cacheTTL       time.Duration
		fetchTimeout   time.Duration
		initialBackoff time.Duration
		maxBackoff     time.Duration
		mirrorzSite    Site
		mirrorzInfo    []Info
	}

	cache atomic.Pointer[cache]

	deps struct {
		logger zerolog.Logger
		hosts  []Host
	}

	ctx     context.Context
	cancel  context.CancelFunc
	sf      singleflight.Group
	backoff time.Duration
}

// cache MUST NOT be modifed once set in manager.
type cache struct {
	mirrors      map[string]Mirror
	sorted       []Mirror
	refreshAfter time.Time
}

func NewManager(cfg ManagerConfig, logger zerolog.Logger) (*Manager, func(), error) {
	// Build hosts
	var hosts []Host

	for _, hcfg := range cfg.Hosts() {
		h, err := BuildHost(hcfg, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("NewManager: %w", err)
		}
		hosts = append(hosts, h)
	}

	ctx, cancel := context.WithCancel(context.Background())

	mgr := &Manager{}

	mgr.cfg.cacheTTL = cfg.CacheTTL()
	mgr.cfg.fetchTimeout = cfg.FetchTimeout()
	mgr.cfg.baseMirrors = cfg.BaseMirrors()
	mgr.cfg.initialBackoff = initialBackoff
	mgr.cfg.maxBackoff = maxBackoff

	if s := cfg.MirrorzSite(); s != nil {
		mgr.cfg.mirrorzSite = *s
	}
	mgr.cfg.mirrorzInfo = cfg.MirrorzInfo()

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = logger.With().Str("module", "mirrors.Manager").Logger()

	mgr.deps.hosts = hosts

	mgr.ctx = ctx
	mgr.cancel = cancel
	mgr.backoff = mgr.cfg.initialBackoff

	return mgr, cancel, nil
}

// All returns an iterator over all mirrors. If the cache is stale and not in
// backoff, it blocks until a fresh fetch completes (or the manager is shut down).
func (mgr *Manager) All() iter.Seq[Mirror] {
	c := mgr.cache.Load()

	if time.Now().After(c.refreshAfter) {
		mgr.deps.logger.Debug().Msg("cache stale, triggering fetch")
		ch := mgr.sf.DoChan("fetch", func() (any, error) {
			mgr.fetch()
			return nil, nil
		})

		select {
		case <-ch:
		case <-mgr.ctx.Done():
			mgr.deps.logger.Debug().Msg("manager context done, skipping fetch")
		}
		c = mgr.cache.Load()
	}

	return func(yield func(Mirror) bool) {
		for _, m := range c.sorted {
			if !yield(m) {
				return
			}
		}
	}
}

func (mgr *Manager) fetch() {
	oldCache := mgr.cache.Load()

	newCache := &cache{
		mirrors: make(map[string]Mirror),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // Protects newCache.mirrors

	ctx, cancel := context.WithTimeout(mgr.ctx, mgr.cfg.fetchTimeout)
	defer cancel()

	allFailed := len(mgr.deps.hosts) > 0

	for _, h := range mgr.deps.hosts {
		wg.Go(func() {
			mirrors, err := h.FetchMirrors(ctx)
			if err != nil {
				mgr.deps.logger.Warn().Err(err).Str("host", h.Name()).Msg("fetch failed")
				return
			}

			mu.Lock()
			allFailed = false

			for _, m := range mirrors {
				if _, ok := newCache.mirrors[m.Name]; ok {
					// TODO: add tie breaker
					mgr.deps.logger.Warn().Str("mirror", m.Name).Msg("duplicate mirror")
					continue
				}

				newCache.mirrors[m.Name] = m
			}
			mu.Unlock()
		})
	}

	wg.Wait()

	// Inject metadata from base mirrors
	for name, meta := range mgr.cfg.baseMirrors {
		m, ok := newCache.mirrors[name]
		if ok {
			m.Metadata = meta.Metadata
			newCache.mirrors[name] = m
		} else {
			newCache.mirrors[name] = meta
		}
	}

	newCache.sorted = make([]Mirror, 0, len(newCache.mirrors))
	for _, m := range newCache.mirrors {
		newCache.sorted = append(newCache.sorted, m)
	}
	slices.SortFunc(newCache.sorted, func(a, b Mirror) int {
		return cmp.Compare(a.Name, b.Name)
	})

	if allFailed {
		if mgr.backoff > mgr.cfg.maxBackoff {
			newCache.refreshAfter = time.Now().Add(mgr.cfg.maxBackoff)
		} else {
			newCache.refreshAfter = time.Now().Add(mgr.backoff)
			mgr.backoff *= 2
		}
		newCache.mirrors = oldCache.mirrors
		newCache.sorted = oldCache.sorted
		mgr.deps.logger.Warn().Dur("backoff", time.Until(newCache.refreshAfter)).Msg("all hosts failed, using stale cache")
	} else {
		mgr.backoff = mgr.cfg.initialBackoff
		newCache.refreshAfter = time.Now().Add(mgr.cfg.cacheTTL)
		mgr.deps.logger.Debug().Int("mirrors", len(newCache.sorted)).Msg("cache refreshed")
	}
	mgr.cache.Store(newCache)
}

// Mirrorz builds the MirrorZ JSON response. If the cache is stale, it triggers
// a fresh upstream fetch (respecting cache TTL) and merges the results into the
// cache so that other consumers benefit from the fresh data.
func (mgr *Manager) Mirrorz() (*Mirrorz, error) {
	c := mgr.cache.Load()

	if time.Now().After(c.refreshAfter) {
		mgr.deps.logger.Debug().Msg("mirrorz: cache stale, triggering fetch")
		ch := mgr.sf.DoChan("fetch", func() (any, error) {
			mgr.fetch()
			return nil, nil
		})

		select {
		case <-ch:
		case <-mgr.ctx.Done():
			mgr.deps.logger.Debug().Msg("mirrorz: manager context done during fetch")
			return nil, mgr.ctx.Err()
		}

		c = mgr.cache.Load()
	}

	entries := make([]MirrorzEntry, 0, len(c.sorted))
	for _, m := range c.sorted {
		entry := MirrorzEntry{
			Cname: m.Name,
		}

		if m.Metadata != nil {
			switch m.Metadata.Type {
			case Rsync:
			case Git:
			case Proxy:
			default:
				continue
			}
			entry.Desc = m.Metadata.Desc
			entry.URL = m.Metadata.URL
			entry.Help = m.Metadata.HelpURL
		}

		if m.Sync != nil {
			entry.Status = BuildMirrorzStatus(m.Sync)
			entry.Upstream = m.Sync.Upstream
			entry.Size = mirrorzSize(m.Sync.Size)
		} else {
			entry.Status = "U"
			entry.Disable = true
		}

		entries = append(entries, entry)
	}

	return &Mirrorz{
		Version: MzVersion,
		Site:    mgr.cfg.mirrorzSite,
		Info:    mgr.cfg.mirrorzInfo,
		Mirrors: entries,
	}, nil
}
