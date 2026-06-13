// LLM usage: generated with deepseek-v4-pro and modified manually.
package mirrors

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockHost implements Host and tracks call count.
type mockHost struct {
	name    string
	mirrors []Mirror
	err     error
	calls   atomic.Int32
	// If blockFetch is set, FetchMirrors blocks until the channel is closed.
	blockFetch chan struct{}
}

func (m *mockHost) Name() string { return m.name }

func (m *mockHost) FetchMirrors(ctx context.Context) ([]Mirror, error) {
	m.calls.Add(1)
	if m.blockFetch != nil {
		select {
		case <-m.blockFetch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.mirrors, nil
}

// mockManagerConfig implements ManagerConfig.
type mockManagerConfig struct {
	hosts        []HostConfig
	cacheTTL     time.Duration
	fetchTimeout time.Duration
	baseMirrors  map[string]Mirror
	mirrorzSite  *Site
	mirrorzInfo  []Info
}

func (m mockManagerConfig) Hosts() []HostConfig            { return m.hosts }
func (m mockManagerConfig) CacheTTL() time.Duration        { return m.cacheTTL }
func (m mockManagerConfig) FetchTimeout() time.Duration    { return m.fetchTimeout }
func (m mockManagerConfig) BaseMirrors() map[string]Mirror { return m.baseMirrors }
func (m mockManagerConfig) MirrorzSite() *Site             { return m.mirrorzSite }
func (m mockManagerConfig) MirrorzInfo() []Info            { return m.mirrorzInfo }

// newTestManager creates a Manager with pre-built hosts for testing.
// Returns the manager and its cancel function.
func newTestManager(hosts []Host, ttl, timeout time.Duration, baseMirrors map[string]Mirror, logger zerolog.Logger) (*Manager, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	mgr := &Manager{}

	mgr.cfg.cacheTTL = ttl
	mgr.cfg.fetchTimeout = timeout
	mgr.cfg.initialBackoff = initialBackoff
	mgr.cfg.maxBackoff = maxBackoff
	mgr.cfg.baseMirrors = baseMirrors

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = logger.With().Str("module", "mirrors.Manager").Logger()
	mgr.deps.hosts = hosts

	mgr.ctx = ctx
	mgr.cancel = cancel
	mgr.backoff = mgr.cfg.initialBackoff

	return mgr, cancel
}

func newTestManagerWithMirrorz(hosts []Host, ttl, timeout time.Duration, baseMirrors map[string]Mirror, site Site, info []Info, logger zerolog.Logger) (*Manager, func()) {
	mgr, cancel := newTestManager(hosts, ttl, timeout, baseMirrors, logger)
	mgr.cfg.mirrorzSite = site
	mgr.cfg.mirrorzInfo = info
	return mgr, cancel
}

func TestManager_All_ReturnsMirrorsFromHost(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{Name: "zzz", Sync: &Sync{Status: Success}},
			{Name: "aaa", Sync: &Sync{Status: Failed}},
			{Name: "mmm", Sync: &Sync{Status: Syncing}},
		},
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0, // TTL=0: always stale, forces fetch on first call
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// First call: cache is stale, blocks until fetch completes.
	mirrors := slices.Collect(mgr.All())

	if len(mirrors) != 3 {
		t.Fatalf("got %d mirrors, want 3", len(mirrors))
	}

	// Mirrors must be sorted by name.
	want := []string{"aaa", "mmm", "zzz"}
	for i, m := range mirrors {
		if m.Name != want[i] {
			t.Errorf("mirrors[%d].Name = %q, want %q", i, m.Name, want[i])
		}
	}

	// Verify Sync is preserved.
	if mirrors[0].Sync == nil || mirrors[0].Sync.Status != Failed {
		t.Errorf("aaa.Status = %v, want 'failed'", mirrors[0].Sync)
	}

	if host.calls.Load() != 1 {
		t.Errorf("FetchMirrors called %d times, want 1", host.calls.Load())
	}
}

func TestManager_All_MergesMultipleHosts(t *testing.T) {
	hostA := &mockHost{
		name: "hA",
		mirrors: []Mirror{
			{Name: "alpine", Sync: &Sync{Status: Success}},
			{Name: "debian", Sync: &Sync{Status: Failed}},
		},
	}
	hostB := &mockHost{
		name: "hB",
		mirrors: []Mirror{
			{Name: "debian", Sync: &Sync{Status: Syncing}},
			{Name: "ubuntu", Sync: &Sync{Status: Success}},
		},
	}

	mgr, cancel := newTestManager(
		[]Host{hostA, hostB},
		0,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	mirrors := slices.Collect(mgr.All())

	// Should have 3 unique mirrors: alpine, debian, ubuntu (sorted).
	if len(mirrors) != 3 {
		t.Fatalf("got %d mirrors, want 3", len(mirrors))
	}

	want := []string{"alpine", "debian", "ubuntu"}
	for i, m := range mirrors {
		if m.Name != want[i] {
			t.Errorf("mirrors[%d].Name = %q, want %q", i, m.Name, want[i])
		}
	}

	if mirrors[0].Sync == nil {
		t.Error("alpine.Sync is nil")
	}
	if mirrors[2].Sync == nil {
		t.Error("ubuntu.Sync is nil")
	}
}

func TestManager_All_HandlesFailedHost(t *testing.T) {
	hostA := &mockHost{
		name:    "hA",
		mirrors: []Mirror{{Name: "alpine"}},
	}
	hostB := &mockHost{
		name: "hB",
		err:  errors.New("fetch failed"),
	}

	mgr, cancel := newTestManager(
		[]Host{hostA, hostB},
		0,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	mirrors := slices.Collect(mgr.All())

	if len(mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1", len(mirrors))
	}
	if mirrors[0].Name != "alpine" {
		t.Errorf("mirrors[0].Name = %q, want %q", mirrors[0].Name, "alpine")
	}
}

func TestManager_All_InjectsBaseMirrorMetadata(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{
				Name: "alpine",
				Sync: &Sync{Status: Success, Size: 4000},
			},
		},
	}

	baseMirrors := map[string]Mirror{
		"alpine": {
			Name: "alpine",
			Metadata: &Metadata{
				Desc:    "Alpine Linux",
				URL:     "https://alpinelinux.org",
				HelpURL: "https://help.example.com/alpine",
			},
		},
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0,
		5*time.Second,
		baseMirrors,
		zerolog.Nop(),
	)
	defer cancel()

	mirrors := slices.Collect(mgr.All())

	if len(mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1", len(mirrors))
	}

	alpine := mirrors[0]
	if alpine.Metadata == nil {
		t.Fatal("alpine.Metadata is nil, base mirror should have injected it")
	}
	if alpine.Metadata.Desc != "Alpine Linux" {
		t.Errorf("alpine.Metadata.Desc = %q, want %q", alpine.Metadata.Desc, "Alpine Linux")
	}
	if alpine.Metadata.URL != "https://alpinelinux.org" {
		t.Errorf("alpine.Metadata.URL = %q", alpine.Metadata.URL)
	}

	if alpine.Sync == nil {
		t.Fatal("alpine.Sync is nil, should be preserved from host")
	}
	if alpine.Sync.Status != Success {
		t.Errorf("alpine.Sync.Status = %v, want %v", alpine.Sync.Status, Success)
	}
}

func TestManager_All_AddsMissingBaseMirrors(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{{Name: "alpine", Sync: &Sync{Status: Success}}},
	}

	baseMirrors := map[string]Mirror{
		"alpine": {
			Name:     "alpine",
			Metadata: &Metadata{Desc: "Alpine Linux"},
		},
		"gentoo": {
			Name:     "gentoo",
			Metadata: &Metadata{Desc: "Gentoo Linux"},
		},
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0,
		5*time.Second,
		baseMirrors,
		zerolog.Nop(),
	)
	defer cancel()

	mirrors := slices.Collect(mgr.All())

	if len(mirrors) != 2 {
		t.Fatalf("got %d mirrors, want 2 (alpine from host + gentoo from base)", len(mirrors))
	}

	if mirrors[0].Name != "alpine" {
		t.Errorf("mirrors[0].Name = %q, want %q", mirrors[0].Name, "alpine")
	}
	if mirrors[1].Name != "gentoo" {
		t.Errorf("mirrors[1].Name = %q, want %q", mirrors[1].Name, "gentoo")
	}

	if mirrors[0].Sync == nil {
		t.Error("alpine.Sync is nil")
	}
	if mirrors[0].Metadata == nil {
		t.Error("alpine.Metadata is nil")
	}

	if mirrors[1].Metadata == nil {
		t.Error("gentoo.Metadata is nil")
	}
	if mirrors[1].Sync != nil {
		t.Error("gentoo.Sync should be nil (not fetched from any host)")
	}
}

func TestManager_All_RespectsCacheTTL(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{{Name: "alpine"}},
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		500*time.Millisecond, // Longer TTL so cache stays fresh.
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// First call: cache stale, blocks and fetches.
	_ = slices.Collect(mgr.All())

	callsAfterFirst := host.calls.Load()

	// Second call immediately: cache is fresh, should NOT trigger another fetch.
	_ = slices.Collect(mgr.All())

	if host.calls.Load() != callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected %d (no extra fetch when cache is fresh)",
			host.calls.Load(), callsAfterFirst)
	}
}

func TestManager_All_RefreshesAfterTTL(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{{Name: "alpine"}},
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		10*time.Millisecond, // Short TTL.
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// First fetch.
	_ = slices.Collect(mgr.All())

	callsAfterFirst := host.calls.Load()

	// Wait for cache to become stale.
	time.Sleep(30 * time.Millisecond)

	// This should trigger a new refresh (blocks until done).
	_ = slices.Collect(mgr.All())

	if host.calls.Load() <= callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected > %d (should refresh after TTL)",
			host.calls.Load(), callsAfterFirst)
	}
}

func TestManager_All_ConcurrentCallersDeduplicated(t *testing.T) {
	// Use blockFetch so we can control when the fetch completes,
	// ensuring all goroutines pile up on the same singleflight call.
	blockFetch := make(chan struct{})
	host := &mockHost{
		name:       "h1",
		mirrors:    []Mirror{{Name: "alpine"}},
		blockFetch: blockFetch,
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	var wg sync.WaitGroup
	const numGoroutines = 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mirrors := slices.Collect(mgr.All())
			if len(mirrors) != 1 || mirrors[0].Name != "alpine" {
				t.Errorf("got unexpected mirrors: %v", mirrors)
			}
		}()
	}

	// Give goroutines time to all reach the singleflight.
	time.Sleep(50 * time.Millisecond)

	// Unblock the fetch.
	close(blockFetch)

	wg.Wait()

	if host.calls.Load() != 1 {
		t.Errorf("FetchMirrors called %d times, want 1 (singleflight dedup)", host.calls.Load())
	}
}

func TestManager_All_BackoffAfterFailure(t *testing.T) {
	host := &mockHost{
		name: "h1",
		err:  errors.New("always fails"),
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// Override backoff to short values for fast testing.
	mgr.cfg.initialBackoff = 50 * time.Millisecond
	mgr.cfg.maxBackoff = 200 * time.Millisecond
	mgr.backoff = mgr.cfg.initialBackoff

	// First call: triggers fetch, host fails, lastAttempt is set.
	_ = slices.Collect(mgr.All())

	callsAfterFirst := host.calls.Load()
	if callsAfterFirst != 1 {
		t.Fatalf("first call: FetchMirrors called %d times, want 1", callsAfterFirst)
	}

	// Second call within backoff: should NOT trigger a new fetch.
	_ = slices.Collect(mgr.All())

	if host.calls.Load() != callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected %d (backoff should prevent re-fetch)",
			host.calls.Load(), callsAfterFirst)
	}

	// Wait past backoff.
	time.Sleep(300 * time.Millisecond)

	// Third call after backoff: should trigger a new fetch.
	_ = slices.Collect(mgr.All())

	if host.calls.Load() <= callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected > %d (should re-fetch after backoff)",
			host.calls.Load(), callsAfterFirst)
	}
}

func TestManager_All_ShutdownReturnsImmediately(t *testing.T) {
	// Host blocks forever — without shutdown, All() would hang.
	host := &mockHost{
		name:       "h1",
		blockFetch: make(chan struct{}),
	}

	mgr, cancel := newTestManager(
		[]Host{host},
		0,
		10*time.Second,
		nil,
		zerolog.Nop(),
	)

	// Cancel (shutdown) the manager before calling All.
	cancel()

	done := make(chan struct{})
	go func() {
		_ = slices.Collect(mgr.All())
		close(done)
	}()

	select {
	case <-done:
		// All() returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("All() did not return after shutdown, expected immediate return")
	}
}

// --- Mirrorz Tests ---

func TestManager_Mirrorz_ReturnsMirrorzWithSync(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{
				Name: "alpine",
				Sync: &Sync{
					Status:       Success,
					LastEnded:    1778201981,
					NextSchedule: 1780703762,
					Upstream:     "rsync://example.com/alpine",
					Size:         4 * 1024 * 1024 * 1024 * 1024,
				},
				Metadata: &Metadata{
					Desc:    "Alpine Linux",
					URL:     "/alpine",
					HelpURL: "/help/alpine",
					Type:    Rsync,
				},
			},
		},
	}

	site := Site{Url: "https://example.org", Abbr: "EX"}
	mgr, cancel := newTestManagerWithMirrorz(
		[]Host{host},
		500*time.Second, // Cache is fresh, but Mirrorz() ignores TTL
		5*time.Second,
		nil,
		site,
		[]Info{{Distro: "Alpine", Category: "os"}},
		zerolog.Nop(),
	)
	defer cancel()

	mz, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if mz.Site.Url != "https://example.org" {
		t.Errorf("Site.Url = %q", mz.Site.Url)
	}
	if len(mz.Info) != 1 || mz.Info[0].Distro != "Alpine" {
		t.Errorf("Info = %+v", mz.Info)
	}
	if len(mz.Mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1", len(mz.Mirrors))
	}

	m := mz.Mirrors[0]
	if m.Cname != "alpine" {
		t.Errorf("Cname = %q, want %q", m.Cname, "alpine")
	}
	if m.Desc != "Alpine Linux" {
		t.Errorf("Desc = %q", m.Desc)
	}
	if m.Url != "/alpine" {
		t.Errorf("Url = %q", m.Url)
	}
	if m.Help != "/help/alpine" {
		t.Errorf("Help = %q", m.Help)
	}
	if m.Upstream != "rsync://example.com/alpine" {
		t.Errorf("Upstream = %q", m.Upstream)
	}
	if m.Status != "S1778201981X1780703762" {
		t.Errorf("Status = %q, want %q", m.Status, "S1778201981X1780703762")
	}
	if m.Size == "" {
		t.Error("Size should not be empty for 4TB")
	}
	if m.Disable {
		t.Error("Disable should be false when Sync is present")
	}
}

func TestManager_Mirrorz_DisableWhenNoSync(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{}, // no mirrors from upstream
	}

	baseMirrors := map[string]Mirror{
		"gentoo": {
			Name:     "gentoo",
			Metadata: &Metadata{Desc: "Gentoo Linux", Type: Rsync},
		},
	}

	site := Site{Url: "https://example.org", Abbr: "EX"}
	mgr, cancel := newTestManagerWithMirrorz(
		[]Host{host},
		500*time.Second,
		5*time.Second,
		baseMirrors,
		site,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	mz, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if len(mz.Mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1 (gentoo from base)", len(mz.Mirrors))
	}

	m := mz.Mirrors[0]
	if m.Cname != "gentoo" {
		t.Errorf("Cname = %q, want %q", m.Cname, "gentoo")
	}
	if !m.Disable {
		t.Error("Disable should be true when Sync is nil (no upstream data)")
	}
	if m.Status != "U" {
		t.Errorf("Status = %q, want %q", m.Status, "U")
	}
}

func TestManager_Mirrorz_RespectsCacheTTL(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{{Name: "alpine"}},
	}

	site := Site{Url: "https://example.org", Abbr: "EX"}
	mgr, cancel := newTestManagerWithMirrorz(
		[]Host{host},
		500*time.Second, // Long TTL: cache stays fresh
		5*time.Second,
		nil,
		site,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	// First Mirrorz call: cache stale, triggers fetch.
	_, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("first Mirrorz() error: %v", err)
	}
	callsAfterFirst := host.calls.Load()

	// Second Mirrorz call: cache is fresh, should NOT trigger another fetch.
	_, err = mgr.Mirrorz()
	if err != nil {
		t.Fatalf("second Mirrorz() error: %v", err)
	}

	if host.calls.Load() != callsAfterFirst {
		t.Errorf("Mirrorz should respect cache TTL; got %d calls, want %d (no extra fetch when cache is fresh)",
			host.calls.Load(), callsAfterFirst)
	}
}

func TestManager_Mirrorz_EmptyMirrors(t *testing.T) {
	host := &mockHost{
		name:    "h1",
		mirrors: []Mirror{},
	}

	site := Site{Url: "https://example.org", Abbr: "EX"}
	mgr, cancel := newTestManagerWithMirrorz(
		[]Host{host},
		0,
		5*time.Second,
		nil,
		site,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	mz, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if len(mz.Mirrors) != 0 {
		t.Errorf("expected 0 mirrors, got %d", len(mz.Mirrors))
	}
}

func TestManager_All_HandlesNilHosts(t *testing.T) {
	mgr, cancel := newTestManager(
		nil, // No hosts.
		0,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)
	defer cancel()

	mirrors := slices.Collect(mgr.All())
	if len(mirrors) != 0 {
		t.Fatalf("got %d mirrors, want 0", len(mirrors))
	}
}
