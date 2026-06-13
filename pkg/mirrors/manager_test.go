// LLM usage: generated with deepseek-v4-pro and modified manually.
package mirrors

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockHost implements Host, signals when FetchMirrors is called,
// and tracks call count for cache TTL tests.
type mockHost struct {
	name    string
	mirrors []Mirror
	err     error
	called  chan struct{}
	calls   atomic.Int32
}

func (m *mockHost) Name() string { return m.name }

func (m *mockHost) FetchMirrors(_ context.Context) ([]Mirror, error) {
	m.calls.Add(1)
	// Signal that we were called (non-blocking).
	select {
	case m.called <- struct{}{}:
	default:
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
}

func (m mockManagerConfig) Hosts() []HostConfig            { return m.hosts }
func (m mockManagerConfig) CacheTTL() time.Duration        { return m.cacheTTL }
func (m mockManagerConfig) FetchTimeout() time.Duration    { return m.fetchTimeout }
func (m mockManagerConfig) BaseMirrors() map[string]Mirror { return m.baseMirrors }

// hostAsConfig adapts a Host to HostConfig for testing.
// Since BuildHost is not used in tests (we inject pre-built hosts via a custom
// manager constructor), this is only needed to satisfy the ManagerConfig interface.
// We use a different approach: create the Manager manually with pre-built hosts.

// newTestManager creates a Manager with pre-built hosts for testing.
func newTestManager(hosts []Host, ttl, timeout time.Duration, baseMirrors map[string]Mirror, logger zerolog.Logger) *Manager {
	mgr := &Manager{}

	mgr.cfg.cacheTTL = ttl
	mgr.cfg.fetchTimeout = timeout
	mgr.cfg.baseMirrors = baseMirrors

	mgr.cache.Store(&cache{
		mirrors: make(map[string]Mirror),
		sorted:  []Mirror{},
	})

	mgr.deps.logger = logger.With().Str("module", "mirrors.Manager").Logger()
	mgr.deps.hosts = hosts

	return mgr
}

func TestManager_All_ReturnsMirrorsFromHost(t *testing.T) {
	called := make(chan struct{}, 1)
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{Name: "zzz", Sync: &Sync{Status: Success}},
			{Name: "aaa", Sync: &Sync{Status: Failed}},
			{Name: "mmm", Sync: &Sync{Status: Syncing}},
		},
		called: called,
	}

	mgr := newTestManager(
		[]Host{host},
		10*time.Millisecond,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)

	// First call: cache is empty (stale), triggers async refresh.
	first := slices.Collect(mgr.All())
	if len(first) != 0 {
		t.Fatalf("first All() got %d mirrors, want 0 (cache not yet populated)", len(first))
	}

	// Wait for the async fetch to be triggered and FetchMirrors to be called.
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for FetchMirrors to be called")
	}

	// Poll until cache is populated (fetch goroutine completes).
	var mirrors []Mirror
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mirrors = slices.Collect(mgr.All())
		if len(mirrors) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

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
}

// waitForMirrors polls All() until mirrors are available or deadline exceeded.
func waitForMirrors(t *testing.T, mgr *Manager) []Mirror {
	t.Helper()
	var mirrors []Mirror
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mirrors = slices.Collect(mgr.All())
		if len(mirrors) > 0 {
			return mirrors
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for mirrors to be available")
	return nil
}

func TestManager_All_MergesMultipleHosts(t *testing.T) {
	hostA := &mockHost{
		name: "hA",
		mirrors: []Mirror{
			{Name: "alpine", Sync: &Sync{Status: Success}},
			{Name: "debian", Sync: &Sync{Status: Failed}},
		},
		called: make(chan struct{}, 1),
	}
	hostB := &mockHost{
		name: "hB",
		mirrors: []Mirror{
			{Name: "debian", Sync: &Sync{Status: Syncing}},
			{Name: "ubuntu", Sync: &Sync{Status: Success}},
		},
		called: make(chan struct{}, 1),
	}

	mgr := newTestManager(
		[]Host{hostA, hostB},
		10*time.Millisecond,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)

	// Trigger async refresh.
	first := slices.Collect(mgr.All())
	if len(first) != 0 {
		t.Fatalf("first All() got %d mirrors, want 0", len(first))
	}

	mirrors := waitForMirrors(t, mgr)

	// Should have 3 unique mirrors: alpine, debian, ubuntu (sorted).
	// Which host's debian wins is non-deterministic due to concurrent fetches.
	if len(mirrors) != 3 {
		t.Fatalf("got %d mirrors, want 3", len(mirrors))
	}

	want := []string{"alpine", "debian", "ubuntu"}
	for i, m := range mirrors {
		if m.Name != want[i] {
			t.Errorf("mirrors[%d].Name = %q, want %q", i, m.Name, want[i])
		}
	}

	// Verify both hosts contributed: alpine from hostA, ubuntu from hostB.
	if mirrors[0].Sync == nil {
		t.Error("alpine.Sync is nil")
	}
	if mirrors[2].Sync == nil {
		t.Error("ubuntu.Sync is nil")
	}
}

func TestManager_All_HandlesFailedHost(t *testing.T) {
	hostA := &mockHost{
		name: "hA",
		mirrors: []Mirror{
			{Name: "alpine"},
		},
		called: make(chan struct{}, 1),
	}
	hostB := &mockHost{
		name:   "hB",
		err:    errors.New("fetch failed"),
		called: make(chan struct{}, 1),
	}

	mgr := newTestManager(
		[]Host{hostA, hostB},
		10*time.Millisecond,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)

	// Trigger async refresh.
	_ = slices.Collect(mgr.All())

	mirrors := waitForMirrors(t, mgr)

	// Only alpine from the successful host.
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
				// No Metadata set — base mirror should inject it.
			},
		},
		called: make(chan struct{}, 1),
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

	mgr := newTestManager(
		[]Host{host},
		10*time.Millisecond,
		5*time.Second,
		baseMirrors,
		zerolog.Nop(),
	)

	// Trigger async refresh.
	_ = slices.Collect(mgr.All())

	mirrors := waitForMirrors(t, mgr)

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

	// Sync from the host must be preserved.
	if alpine.Sync == nil {
		t.Fatal("alpine.Sync is nil, should be preserved from host")
	}
	if alpine.Sync.Status != Success {
		t.Errorf("alpine.Sync.Status = %v, want %v", alpine.Sync.Status, Success)
	}
}

func TestManager_All_AddsMissingBaseMirrors(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{Name: "alpine", Sync: &Sync{Status: Success}},
		},
		called: make(chan struct{}, 1),
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

	mgr := newTestManager(
		[]Host{host},
		10*time.Millisecond,
		5*time.Second,
		baseMirrors,
		zerolog.Nop(),
	)

	_ = slices.Collect(mgr.All())

	mirrors := waitForMirrors(t, mgr)

	if len(mirrors) != 2 {
		t.Fatalf("got %d mirrors, want 2 (alpine from host + gentoo from base)", len(mirrors))
	}

	// Sorted: alpine, gentoo.
	if mirrors[0].Name != "alpine" {
		t.Errorf("mirrors[0].Name = %q, want %q", mirrors[0].Name, "alpine")
	}
	if mirrors[1].Name != "gentoo" {
		t.Errorf("mirrors[1].Name = %q, want %q", mirrors[1].Name, "gentoo")
	}

	// alpine: has both Sync (from host) and Metadata (from base).
	if mirrors[0].Sync == nil {
		t.Error("alpine.Sync is nil")
	}
	if mirrors[0].Metadata == nil {
		t.Error("alpine.Metadata is nil")
	}

	// gentoo: has Metadata (from base) but no Sync (not fetched).
	if mirrors[1].Metadata == nil {
		t.Error("gentoo.Metadata is nil")
	}
	if mirrors[1].Sync != nil {
		t.Error("gentoo.Sync should be nil (not fetched from any host)")
	}
}

func TestManager_All_RespectsCacheTTL(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{Name: "alpine"},
		},
		called: make(chan struct{}, 10),
	}

	mgr := newTestManager(
		[]Host{host},
		10*time.Millisecond,
		5*time.Second,
		nil,
		zerolog.Nop(),
	)

	// First call: triggers refresh (cache stale).
	_ = slices.Collect(mgr.All())
	<-host.called // Wait for fetch to be triggered.

	// Wait for cache to be populated.
	_ = waitForMirrors(t, mgr)

	callsAfterFirst := host.calls.Load()

	// Second call immediately: cache is fresh, should NOT trigger another fetch.
	_ = slices.Collect(mgr.All())

	// Give a tiny bit of time for any unexpected async fetch to start.
	time.Sleep(5 * time.Millisecond)

	if host.calls.Load() != callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected %d (no extra fetch when cache is fresh)",
			host.calls.Load(), callsAfterFirst)
	}
}

func TestManager_All_RefreshesAfterTTL(t *testing.T) {
	host := &mockHost{
		name: "h1",
		mirrors: []Mirror{
			{Name: "alpine"},
		},
		called: make(chan struct{}, 10),
	}

	mgr := newTestManager(
		[]Host{host},
		10*time.Millisecond, // Short TTL for testing.
		5*time.Second,
		nil,
		zerolog.Nop(),
	)

	// First fetch.
	_ = slices.Collect(mgr.All())
	<-host.called
	_ = waitForMirrors(t, mgr)

	callsAfterFirst := host.calls.Load()

	// Wait for cache to become stale.
	time.Sleep(30 * time.Millisecond)

	// This should trigger a new refresh.
	_ = slices.Collect(mgr.All())
	<-host.called
	_ = waitForMirrors(t, mgr)

	if host.calls.Load() <= callsAfterFirst {
		t.Errorf("FetchMirrors called %d times, expected > %d (should refresh after TTL)",
			host.calls.Load(), callsAfterFirst)
	}
}
