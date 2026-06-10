package syncstatus

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openana/prism/pkg/meta"
)

type TunasyncMirror Mirror // Same definition for now.

type TunasyncManagerConfig interface {
	Upstreams() []string
	CacheTTL() time.Duration
	FetchTimeout() time.Duration
}

type TunasyncManager struct {
	cfg struct {
		upstreams    []string
		cacheTTL     time.Duration
		fetchTimeout time.Duration
	}

	cache atomic.Pointer[cache]

	deps struct {
		logger *slog.Logger
	}

	refreshing atomic.Bool
}

// cache MUST NOT be modifed once set in manager.
type cache struct {
	mirrors    map[string]Mirror
	sorted     []Mirror
	lastUpdate time.Time
}

func NewTunasyncManager(cfg TunasyncManagerConfig, baseLogger *slog.Logger) (*TunasyncManager, error) {
	mgr := &TunasyncManager{}

	mgr.cfg.cacheTTL = cfg.CacheTTL()
	mgr.cfg.fetchTimeout = cfg.FetchTimeout()

	// Validate upstream URLs.
	for _, u := range cfg.Upstreams() {
		if !isWebURL(u) {
			return nil, fmt.Errorf("NewTunasyncManager: invalid upstream %q", u)
		}
		mgr.cfg.upstreams = append(mgr.cfg.upstreams, u)
	}

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = baseLogger.With("component", "TunasyncManager")

	return mgr, nil
}

func (mgr *TunasyncManager) fetchUpstreams() {
	newCache := &cache{
		mirrors: make(map[string]Mirror),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // Protects newCache.mirrors

	ctx, cancel := context.WithTimeout(context.Background(), mgr.cfg.fetchTimeout)
	defer cancel()

	for _, u := range mgr.cfg.upstreams {
		wg.Go(func() {
			mirrors, err := fetchSingleUpstream(ctx, u)
			if err != nil {
				mgr.deps.logger.Warn("fetch upstream failed", "error", err, "upstream", u)
			}

			mu.Lock()
			defer mu.Unlock()

			for _, m := range mirrors {
				if _, ok := newCache.mirrors[m.Name]; ok {
					mgr.deps.logger.Warn("duplicate mirror", "mirror", m.Name)
					continue
				}

				newCache.mirrors[m.Name] = Mirror(m)
			}
		})
	}

	wg.Wait()

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

func fetchSingleUpstream(ctx context.Context, u string) ([]TunasyncMirror, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", meta.UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var mirrors []TunasyncMirror

	if err := json.NewDecoder(resp.Body).Decode(&mirrors); err != nil {
		return nil, err
	}

	return mirrors, nil
}

func (mgr *TunasyncManager) refreshIfStale(lastUpdate time.Time) {
	if time.Since(lastUpdate) < mgr.cfg.cacheTTL {
		return
	}

	if !mgr.refreshing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer mgr.refreshing.Store(false)
		mgr.fetchUpstreams()
	}()
}

func (mgr *TunasyncManager) All() iter.Seq[Mirror] {
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

func (mgr *TunasyncManager) Get(name string) (m Mirror, ok bool) {
	c := mgr.cache.Load()

	mgr.refreshIfStale(c.lastUpdate)

	m, ok = c.mirrors[name]
	return
}

func isWebURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.Host == "" {
		return false
	}

	return true
}
