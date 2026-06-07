// LLM usage: generated with deepseek-v4-pro and modified manually.
package statemgr

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// =============================================================================
// mock config
// =============================================================================

type mockTunasyncConfig struct {
	name           string
	prefix         string
	upstreams      []string
	updateInterval time.Duration
	fetchTimeout   time.Duration
}

func (m mockTunasyncConfig) Name() string                  { return m.name }
func (m mockTunasyncConfig) Prefix() string                { return m.prefix }
func (m mockTunasyncConfig) Upstreams() []string           { return m.upstreams }
func (m mockTunasyncConfig) UpdateInterval() time.Duration { return m.updateInterval }
func (m mockTunasyncConfig) FetchTimeout() time.Duration   { return m.fetchTimeout }

// =============================================================================
// helpers
// =============================================================================

// startFakeUpstream starts an httptest server that returns mirrors as JSON.
// If delay > 0, the handler sleeps before responding (used for timeout tests).
func startFakeUpstream(t *testing.T, mirrors []Mirror, statusCode int, delay time.Duration) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(newFakeUpstreamHandler(t, mirrors, statusCode, delay))
	t.Cleanup(server.Close)
	return server
}

// newFakeUpstreamHandler returns an http.Handler that serves mirror data.
func newFakeUpstreamHandler(t *testing.T, mirrors []Mirror, statusCode int, delay time.Duration) *fakeUpstreamHandler {
	t.Helper()
	return &fakeUpstreamHandler{
		t:          t,
		mirrors:    mirrors,
		statusCode: statusCode,
		delay:      delay,
	}
}

type fakeUpstreamHandler struct {
	t          *testing.T
	mirrors    []Mirror
	statusCode int
	delay      time.Duration
}

func (h *fakeUpstreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-r.Context().Done():
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.statusCode)

	if err := json.NewEncoder(w).Encode(h.mirrors); err != nil {
		h.t.Errorf("fake upstream: failed to encode mirrors: %v", err)
	}
}

// loadAllMirrors reads mirrors from testdata/mirrors.json.
func loadAllMirrors(t *testing.T) []Mirror {
	t.Helper()

	data, err := os.ReadFile("testdata/mirrors.json")
	if err != nil {
		t.Fatalf("failed to read testdata/mirrors.json: %v", err)
	}

	var mirrors []Mirror
	if err := json.Unmarshal(data, &mirrors); err != nil {
		t.Fatalf("failed to unmarshal mirrors: %v", err)
	}

	return mirrors
}

// filterMirrorsByNames returns mirrors whose names are in the allowed set.
func filterMirrorsByNames(t *testing.T, all []Mirror, names ...string) []Mirror {
	t.Helper()

	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}

	var result []Mirror
	for _, m := range all {
		if allowed[m.Name] {
			result = append(result, m)
		}
	}

	return result
}

// mustNewManager creates a TunasyncStateManager with captured log output.
func mustNewManager(t *testing.T, cfg TunasyncStateManagerConfig, logBuf *bytes.Buffer) *TunasyncStateManager {
	t.Helper()

	var logger *slog.Logger
	if logBuf != nil {
		logger = slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}

	mgr, err := NewTunasyncStateManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewTunasyncStateManager() unexpected error: %v", err)
	}
	return mgr
}

// waitForFirstFetch polls GetAllMirrors until lastUpdate is non-zero or deadline is reached.
func waitForFirstFetch(t *testing.T, mgr *TunasyncStateManager, deadline time.Duration) {
	t.Helper()

	timeout := time.After(deadline)
	for {
		_, lastUpdate := mgr.GetAllMirrors()
		if !lastUpdate.IsZero() {
			return
		}
		select {
		case <-timeout:
			t.Fatal("timed out waiting for first fetch cycle")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// stopManager stops the manager and fails the test on error.
func stopManager(t *testing.T, mgr *TunasyncStateManager) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("Stop() unexpected error: %v", err)
	}
}

// =============================================================================
// Constructor tests
// =============================================================================

func TestNewTunasyncStateManager_Valid(t *testing.T) {
	cfg := mockTunasyncConfig{
		name:           "test-tunasync",
		prefix:         "/",
		upstreams:      []string{"https://example.com/mirrors", "https://other.example.com/mirrors"},
		updateInterval: 10 * time.Minute,
		fetchTimeout:   30 * time.Second,
	}

	mgr, err := NewTunasyncStateManager(cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewTunasyncStateManager() unexpected error: %v", err)
	}

	// Verify initial cache is empty.
	mirrors, lastUpdate := mgr.GetAllMirrors()
	if len(mirrors) != 0 {
		t.Errorf("GetAllMirrors() len = %d, want 0", len(mirrors))
	}
	if !lastUpdate.IsZero() {
		t.Errorf("GetAllMirrors() lastUpdate = %v, want zero", lastUpdate)
	}
}

func TestNewTunasyncStateManager_InvalidUpstream(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
	}{
		{"ftp scheme", "ftp://example.com/mirrors"},
		{"empty host", "http:///mirrors"},
		{"no scheme", "example.com/mirrors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mockTunasyncConfig{
				name:           "test",
				prefix:         "/",
				upstreams:      []string{tt.upstream},
				updateInterval: time.Minute,
				fetchTimeout:   time.Second,
			}

			_, err := NewTunasyncStateManager(cfg, slog.Default())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// =============================================================================
// Lifecycle tests
// =============================================================================

func TestRunAndStop_CleanShutdown(t *testing.T) {
	all := loadAllMirrors(t)

	server := startFakeUpstream(t, all[:3], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Wait for at least one fetch cycle.
	waitForFirstFetch(t, mgr, 2*time.Second)

	// Stop cleanly.
	stopManager(t, mgr)

	// Verify we got data.
	mirrors, _ := mgr.GetAllMirrors()
	if len(mirrors) != 3 {
		t.Errorf("GetAllMirrors() len = %d, want 3", len(mirrors))
	}
}

func TestStop_AlreadyCanceledContext(t *testing.T) {
	all := loadAllMirrors(t)

	server := startFakeUpstream(t, all[:1], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: time.Hour, // effectively never tick
		fetchTimeout:   time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Stop with an already-canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.Stop(ctx)
	// Either nil (done closed first) or context.Canceled (ctx fired first) is acceptable.
	if err != nil && err != context.Canceled {
		t.Errorf("Stop() error = %v, want nil or context.Canceled", err)
	}
}

// =============================================================================
// Data-fetching tests
// =============================================================================

func TestMultipleUpstreams(t *testing.T) {
	all := loadAllMirrors(t)

	// Split mirrors into two disjoint sets.
	upstreamAMirrors := all[:5]   // alpine, archlinux, archlinuxarm, archlinuxcn, blackarch
	upstreamBMirrors := all[5:10] // brew.git, centos, centos-vault, ceph, cpan

	serverA := startFakeUpstream(t, upstreamAMirrors, 200, 0)
	serverB := startFakeUpstream(t, upstreamBMirrors, 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{serverA.URL, serverB.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	var logBuf bytes.Buffer
	mgr := mustNewManager(t, cfg, &logBuf)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()

	expectedCount := len(upstreamAMirrors) + len(upstreamBMirrors)
	if len(mirrors) != expectedCount {
		t.Errorf("GetAllMirrors() len = %d, want %d", len(mirrors), expectedCount)
	}

	// Verify all expected names are present.
	gotNames := make(map[string]bool, len(mirrors))
	for _, m := range mirrors {
		gotNames[m.Name] = true
	}
	for _, m := range upstreamAMirrors {
		if !gotNames[m.Name] {
			t.Errorf("missing mirror %q from upstream A", m.Name)
		}
	}
	for _, m := range upstreamBMirrors {
		if !gotNames[m.Name] {
			t.Errorf("missing mirror %q from upstream B", m.Name)
		}
	}
}

func TestDuplicateMirrorsAcrossUpstreams(t *testing.T) {
	all := loadAllMirrors(t)

	// Both upstreams include the same mirror (alpine).
	shared := filterMirrorsByNames(t, all, "alpine")
	upstreamASet := append(shared, filterMirrorsByNames(t, all, "archlinux", "centos")...)
	upstreamBSet := append(shared, filterMirrorsByNames(t, all, "debian", "fedora")...)

	serverA := startFakeUpstream(t, upstreamASet, 200, 0)
	serverB := startFakeUpstream(t, upstreamBSet, 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{serverA.URL, serverB.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	var logBuf bytes.Buffer
	mgr := mustNewManager(t, cfg, &logBuf)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()

	// We should have 5 unique mirrors, not 6.
	uniqueNames := make(map[string]bool)
	for _, m := range mirrors {
		uniqueNames[m.Name] = true
	}
	if len(uniqueNames) != 5 {
		t.Errorf("unique mirrors count = %d, want 5 (duplicate should be deduplicated)", len(uniqueNames))
	}

	// Verify the duplicate warning was logged.
	if logBuf.Len() == 0 {
		t.Error("expected duplicate mirror warning log, got none")
	}
	if !bytes.Contains(logBuf.Bytes(), []byte("duplicate mirror")) {
		t.Errorf("log output does not contain 'duplicate mirror': %s", logBuf.String())
	}
}

func TestGetMirror(t *testing.T) {
	all := loadAllMirrors(t)

	server := startFakeUpstream(t, all[:3], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	// Look up an existing mirror.
	mirror, lastUpdate, ok := mgr.GetMirror(all[0].Name)
	if !ok {
		t.Fatalf("GetMirror(%q) ok = false, want true", all[0].Name)
	}
	if mirror.Name != all[0].Name {
		t.Errorf("GetMirror() Name = %q, want %q", mirror.Name, all[0].Name)
	}
	if lastUpdate.IsZero() {
		t.Error("GetMirror() lastUpdate is zero")
	}

	// Look up a non-existent mirror.
	_, _, ok = mgr.GetMirror("nonexistent-mirror")
	if ok {
		t.Error("GetMirror('nonexistent-mirror') ok = true, want false")
	}
}

func TestGetAllMirrorsReturnsIndependentCopy(t *testing.T) {
	all := loadAllMirrors(t)

	server := startFakeUpstream(t, all[:3], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	// Get first copy.
	mirrors1, _ := mgr.GetAllMirrors()
	origLen := len(mirrors1)

	// Mutate the returned slice.
	if len(mirrors1) > 0 {
		mirrors1[0] = Mirror{Name: "corrupted"}
	}
	mirrors1 = append(mirrors1, Mirror{Name: "injected"})

	// Get second copy — must be unaffected.
	mirrors2, _ := mgr.GetAllMirrors()
	if len(mirrors2) != origLen {
		t.Errorf("GetAllMirrors() len = %d after mutation, want %d", len(mirrors2), origLen)
	}
	for _, m := range mirrors2 {
		if m.Name == "corrupted" || m.Name == "injected" {
			t.Errorf("GetAllMirrors() returned mutated/corrupted data: %q", m.Name)
		}
	}
}

func TestGetAllMirrorsSorted(t *testing.T) {
	all := loadAllMirrors(t)

	// Pick mirrors in non-alphabetical order: centos, alpine, debian.
	unsorted := filterMirrorsByNames(t, all, "centos", "alpine", "debian")
	server := startFakeUpstream(t, unsorted, 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()

	if len(mirrors) != 3 {
		t.Fatalf("GetAllMirrors() len = %d, want 3", len(mirrors))
	}

	// Verify ascending order by Name.
	want := []string{"alpine", "centos", "debian"}
	for i, m := range mirrors {
		if m.Name != want[i] {
			t.Errorf("GetAllMirrors()[%d].Name = %q, want %q", i, m.Name, want[i])
		}
	}
}

// =============================================================================
// Error resilience tests
// =============================================================================

func TestUpstreamReturnsErrorStatus(t *testing.T) {
	all := loadAllMirrors(t)

	// Upstream A returns 500, upstream B works fine.
	serverA := startFakeUpstream(t, all[:3], 500, 0)
	serverB := startFakeUpstream(t, all[5:8], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{serverA.URL, serverB.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	var logBuf bytes.Buffer
	mgr := mustNewManager(t, cfg, &logBuf)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()

	// Only mirrors from the healthy upstream should appear.
	if len(mirrors) != 3 {
		t.Errorf("GetAllMirrors() len = %d, want 3 (only from healthy upstream)", len(mirrors))
	}

	// Verify warning was logged for the failed upstream.
	if !bytes.Contains(logBuf.Bytes(), []byte("fetch upstream failed")) {
		t.Errorf("log output does not contain 'fetch upstream failed': %s", logBuf.String())
	}
}

func TestUpstreamReturnsMalformedJSON(t *testing.T) {
	// Create a server that returns non-JSON content.
	badServer := httptest.NewServer(newRawHandler(200, "this is not json"))
	t.Cleanup(badServer.Close)

	all := loadAllMirrors(t)
	goodServer := startFakeUpstream(t, all[:3], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{badServer.URL, goodServer.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	var logBuf bytes.Buffer
	mgr := mustNewManager(t, cfg, &logBuf)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()
	if len(mirrors) != 3 {
		t.Errorf("GetAllMirrors() len = %d, want 3 (from healthy upstream)", len(mirrors))
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("fetch upstream failed")) {
		t.Errorf("log output does not contain 'fetch upstream failed': %s", logBuf.String())
	}
}

func TestUpstreamTimeout(t *testing.T) {
	all := loadAllMirrors(t)

	// Server A responds with a long delay, server B responds normally.
	serverA := startFakeUpstream(t, all[:3], 200, 5*time.Second)
	serverB := startFakeUpstream(t, all[5:8], 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{serverA.URL, serverB.URL},
		updateInterval: 50 * time.Millisecond,
		fetchTimeout:   100 * time.Millisecond, // shorter than server A's delay
	}

	var logBuf bytes.Buffer
	mgr := mustNewManager(t, cfg, &logBuf)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()

	// Only mirrors from the responsive upstream should appear.
	if len(mirrors) != 3 {
		t.Errorf("GetAllMirrors() len = %d, want 3 (from responsive upstream)", len(mirrors))
	}

	// Verify timeout warning was logged.
	if !bytes.Contains(logBuf.Bytes(), []byte("fetch upstream failed")) {
		t.Errorf("log output does not contain 'fetch upstream failed': %s", logBuf.String())
	}
}

// =============================================================================
// Edge case tests
// =============================================================================

func TestEmptyUpstreamList(t *testing.T) {
	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      nil,
		updateInterval: time.Minute,
		fetchTimeout:   time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	mirrors, lastUpdate := mgr.GetAllMirrors()
	if len(mirrors) != 0 {
		t.Errorf("GetAllMirrors() len = %d, want 0", len(mirrors))
	}
	if !lastUpdate.IsZero() {
		t.Errorf("GetAllMirrors() lastUpdate = %v, want zero", lastUpdate)
	}
}

func TestUpstreamReturnsEmptyArray(t *testing.T) {
	server := startFakeUpstream(t, nil, 200, 0)

	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{server.URL},
		updateInterval: 20 * time.Millisecond,
		fetchTimeout:   5 * time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	defer stopManager(t, mgr)

	waitForFirstFetch(t, mgr, 2*time.Second)

	mirrors, _ := mgr.GetAllMirrors()
	if len(mirrors) != 0 {
		t.Errorf("GetAllMirrors() len = %d, want 0", len(mirrors))
	}
}

func TestStopBeforeRun(t *testing.T) {
	cfg := mockTunasyncConfig{
		name:           "test",
		prefix:         "/",
		upstreams:      []string{"https://example.com/mirrors"},
		updateInterval: time.Minute,
		fetchTimeout:   time.Second,
	}

	mgr := mustNewManager(t, cfg, nil)

	// Stop without Run should return nil (cancel is nil, early return).
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := mgr.Stop(ctx); err != nil {
		t.Errorf("Stop() before Run() unexpected error: %v", err)
	}
}

// =============================================================================
// rawHandler — serves a fixed raw string as response body
// =============================================================================

type rawHandler struct {
	statusCode int
	body       string
}

func newRawHandler(statusCode int, body string) *rawHandler {
	return &rawHandler{statusCode: statusCode, body: body}
}

func (h *rawHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(h.statusCode)
	_, _ = w.Write([]byte(h.body))
}
