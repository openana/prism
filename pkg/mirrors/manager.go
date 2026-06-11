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
)

type ManagerConfig interface {
	Hosts() []HostConfig
	CacheTTL() time.Duration
	FetchTimeout() time.Duration
	BaseMirrors() map[string]Mirror
}

type Manager struct {
	cfg struct {
		cacheTTL     time.Duration
		fetchTimeout time.Duration
		baseMirrors  map[string]Mirror
	}

	cache atomic.Pointer[cache]

	deps struct {
		logger zerolog.Logger
		hosts  []Host
	}

	fetching atomic.Bool
}

// cache MUST NOT be modifed once set in manager.
type cache struct {
	mirrors    map[string]Mirror
	sorted     []Mirror
	lastUpdate time.Time
}

func NewManager(cfg ManagerConfig, logger zerolog.Logger) (*Manager, error) {
	// Build hosts
	var hosts []Host

	for _, hcfg := range cfg.Hosts() {
		h, err := BuildHost(hcfg, logger)
		if err != nil {
			return nil, fmt.Errorf("NewManager: %w", err)
		}
		hosts = append(hosts, h)
	}

	mgr := &Manager{}

	mgr.cfg.cacheTTL = cfg.CacheTTL()
	mgr.cfg.fetchTimeout = cfg.FetchTimeout()
	mgr.cfg.baseMirrors = cfg.BaseMirrors()

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = logger.With().Str("module", "mirrors.Manager").Logger()

	mgr.deps.hosts = hosts

	return mgr, nil
}

// Does not guarantee info younger than ttl.
func (mgr *Manager) All() iter.Seq[Mirror] {
	c := mgr.cache.Load()

	mgr.refreshIfStale(c.lastUpdate)

	return func(yield func(Mirror) bool) {
		for _, m := range c.sorted {
			if !yield(m) {
				return
			}
		}
	}
}

func (mgr *Manager) refreshIfStale(lastUpdate time.Time) {
	if time.Since(lastUpdate) < mgr.cfg.cacheTTL {
		return
	}

	if !mgr.fetching.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer mgr.fetching.Store(false)
		mgr.fetch()
	}()
}

func (mgr *Manager) fetch() {
	newCache := &cache{
		mirrors: make(map[string]Mirror),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // Protects newCache.mirrors

	ctx, cancel := context.WithTimeout(context.Background(), mgr.cfg.fetchTimeout)
	defer cancel()

	for _, h := range mgr.deps.hosts {
		wg.Go(func() {
			mirrors, err := h.FetchMirrors(ctx)
			if err != nil {
				mgr.deps.logger.Warn().Err(err).Str("host", h.Name()).Msg("fetch failed")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			for _, m := range mirrors {
				if _, ok := newCache.mirrors[m.Name]; ok {
					// TODO: add tie breaker
					mgr.deps.logger.Warn().Str("mirror", m.Name).Msg("duplicate mirror")
					continue
				}

				newCache.mirrors[m.Name] = m
			}
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

	newCache.lastUpdate = time.Now()

	mgr.cache.Store(newCache)
}
