package statemgr

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"time"

	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/module"
)

type TunasyncMirror Mirror // Same definition for now.

type TunasyncStateManagerConfig interface {
	module.ModuleConfig
	Upstreams() []string
	UpdateInterval() time.Duration
	FetchTimeout() time.Duration
}

type TunasyncStateManager struct {
	cfg struct {
		upstreams      []string
		updateInterval time.Duration
		fetchTimeout   time.Duration
	}

	cache atomic.Pointer[cache]

	deps struct {
		logger *slog.Logger
	}

	workers struct {
		done   chan struct{}
		cancel context.CancelFunc
	}
}

// cache MUST NOT be modifed once set in manager.
type cache struct {
	mirrors    map[string]Mirror
	sorted     []Mirror
	lastUpdate time.Time
}

func NewTunasyncStateManager(cfg TunasyncStateManagerConfig, baseLogger *slog.Logger) (*TunasyncStateManager, error) {
	mgr := &TunasyncStateManager{}

	mgr.cfg.updateInterval = cfg.UpdateInterval()
	mgr.cfg.fetchTimeout = cfg.FetchTimeout()

	// Validate upstream URLs.
	for _, u := range cfg.Upstreams() {
		if !isWebURL(u) {
			return nil, fmt.Errorf("NewTunasyncStateManager: invalid upstream %q", u)
		}
		mgr.cfg.upstreams = append(mgr.cfg.upstreams, u)
	}

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = baseLogger.With("component", "TunasyncStateManager")

	mgr.workers.done = make(chan struct{})

	return mgr, nil
}

func (mgr *TunasyncStateManager) Run(ctx context.Context) error {
	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	mgr.workers.cancel = cancel

	go mgr.routineFetchUpstream(ctx)

	return nil
}

func (mgr *TunasyncStateManager) Stop(ctx context.Context) error {
	if mgr.workers.cancel == nil {
		return nil
	}
	mgr.workers.cancel()

	select {
	case <-mgr.workers.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (mgr *TunasyncStateManager) routineFetchUpstream(ctx context.Context) {
	defer close(mgr.workers.done)
	ticker := time.NewTicker(mgr.cfg.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newCache := &cache{
				mirrors: make(map[string]Mirror),
			}

			for _, u := range mgr.cfg.upstreams {
				if ctx.Err() != nil {
					break
				}
				mirrors, err := func() ([]TunasyncMirror, error) {
					timeoutCtx, cancel := context.WithTimeout(ctx, mgr.cfg.fetchTimeout)
					defer cancel()

					req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, u, nil)
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
				}()

				if err != nil {
					mgr.deps.logger.Warn("fetch upstream failed", "error", err, "upstream", u)
					continue
				}

				for _, m := range mirrors {
					if _, ok := newCache.mirrors[m.Name]; ok {
						mgr.deps.logger.Warn("duplicate mirror", "mirror", m.Name)
						continue
					}

					newCache.mirrors[m.Name] = Mirror(m)
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
	}
}

func (mgr *TunasyncStateManager) GetAllMirrors() (ms []Mirror, lastUpdate time.Time) {
	c := mgr.cache.Load()
	return slices.Clone(c.sorted), c.lastUpdate
}

func (mgr *TunasyncStateManager) GetMirror(name string) (m Mirror, lastUpdate time.Time, ok bool) {
	cache := mgr.cache.Load()

	lastUpdate = cache.lastUpdate
	m, ok = cache.mirrors[name]
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
