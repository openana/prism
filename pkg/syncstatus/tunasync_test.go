// LLM usage: generated with deepseek-v4-pro and modified manually.
package syncstatus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockTunasyncConfig implements TunasyncManagerConfig.
type mockTunasyncConfig struct {
	upstreams    []string
	cacheTTL     time.Duration
	fetchTimeout time.Duration
}

func (m mockTunasyncConfig) Upstreams() []string         { return m.upstreams }
func (m mockTunasyncConfig) CacheTTL() time.Duration     { return m.cacheTTL }
func (m mockTunasyncConfig) FetchTimeout() time.Duration { return m.fetchTimeout }

func TestNewTunasyncManager_InvalidUpstream(t *testing.T) {
	tests := []struct {
		name      string
		upstream  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty string",
			upstream:  "",
			wantErr:   true,
			errSubstr: "invalid upstream",
		},
		{
			name:      "missing scheme",
			upstream:  "example.com/api",
			wantErr:   true,
			errSubstr: "invalid upstream",
		},
		{
			name:      "ftp scheme",
			upstream:  "ftp://example.com/api",
			wantErr:   true,
			errSubstr: "invalid upstream",
		},
		{
			name:      "missing host",
			upstream:  "https:///api",
			wantErr:   true,
			errSubstr: "invalid upstream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mockTunasyncConfig{
				upstreams:    []string{tt.upstream},
				cacheTTL:     time.Minute,
				fetchTimeout: 10 * time.Second,
			}

			_, err := NewTunasyncManager(cfg, zerolog.New(io.Discard))
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewTunasyncManager() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
			}
		})
	}
}

func TestNewTunasyncManager_EmptyCache(t *testing.T) {
	cfg := mockTunasyncConfig{
		upstreams:    []string{"https://example.com/tunasync"},
		cacheTTL:     time.Minute,
		fetchTimeout: 10 * time.Second,
	}

	mgr, err := NewTunasyncManager(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewTunasyncManager() unexpected error: %v", err)
	}

	// All() should yield nothing when cache is empty.
	for range mgr.All() {
		t.Error("All() yielded items on empty cache, want none")
	}

	// Get() should return not-found for any name.
	if _, ok := mgr.Get("anything"); ok {
		t.Error("Get() returned ok=true on empty cache, want false")
	}
}

// pollMirrors keeps calling All() until mirrors appear or timeout.
func pollMirrors(t *testing.T, mgr *TunasyncManager, timeout time.Duration) []Mirror {
	t.Helper()
	deadline := time.After(timeout)
	for {
		var mirrors []Mirror
		for m := range mgr.All() {
			mirrors = append(mirrors, m)
		}
		if len(mirrors) > 0 {
			return mirrors
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for mirrors after %v", timeout)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestTunasyncManager_All_SortedByName(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()

	mirrors := pollMirrors(t, mgr, 5*time.Second)

	if len(mirrors) == 0 {
		t.Fatal("All() returned no mirrors after fetch")
	}

	// Verify sorted by name.
	for i := 1; i < len(mirrors); i++ {
		if mirrors[i-1].Name >= mirrors[i].Name {
			t.Errorf("mirrors not sorted: %q >= %q at index %d",
				mirrors[i-1].Name, mirrors[i].Name, i)
		}
	}

	// Spot-check a known mirror from the fixture.
	found := false
	for _, m := range mirrors {
		if m.Name == "alpine" {
			found = true
			if m.Status != "failed" {
				t.Errorf("alpine status = %q, want %q", m.Status, "failed")
			}
			break
		}
	}
	if !found {
		t.Error("expected mirror 'alpine' not found in results")
	}
}

// testLogger returns a logger that discards output, for clean test runs.
func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

// newTestManager starts a test HTTP server serving tunasync.json and returns a
// manager configured to fetch from it. The caller is responsible for closing
// the server via the returned cleanup function.
func newTestManager(t *testing.T) (*TunasyncManager, func()) {
	t.Helper()

	data, err := os.ReadFile("testdata/tunasync.json")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))

	cfg := mockTunasyncConfig{
		upstreams:    []string{srv.URL},
		cacheTTL:     0, // Always stale → trigger refresh on every call.
		fetchTimeout: 5 * time.Second,
	}

	mgr, err := NewTunasyncManager(cfg, testLogger())
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}

	return mgr, func() { srv.Close() }
}

func TestTunasyncManager_Get_Found(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()

	// Poll until the cache is populated.
	pollMirrors(t, mgr, 5*time.Second)

	m, ok := mgr.Get("alpine")
	if !ok {
		t.Fatal("Get(alpine) returned ok=false, want true")
	}
	if m.Name != "alpine" {
		t.Errorf("Name = %q, want alpine", m.Name)
	}
	if m.Status != "failed" {
		t.Errorf("Status = %q, want failed", m.Status)
	}
}

func TestTunasyncManager_Get_NotFound(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()

	// Poll until the cache is populated.
	pollMirrors(t, mgr, 5*time.Second)

	if _, ok := mgr.Get("nonexistent-mirror"); ok {
		t.Error("Get(nonexistent-mirror) returned ok=true, want false")
	}
}

func TestTunasyncManager_All_DuplicatesFirstWins(t *testing.T) {
	data, err := os.ReadFile("testdata/tunasync.json")
	if err != nil {
		t.Fatal(err)
	}

	// Two test servers serving the same fixture.
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv2.Close()

	cfg := mockTunasyncConfig{
		upstreams:    []string{srv1.URL, srv2.URL},
		cacheTTL:     0,
		fetchTimeout: 5 * time.Second,
	}

	mgr, err := NewTunasyncManager(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	mirrors := pollMirrors(t, mgr, 5*time.Second)

	// Each mirror name should appear exactly once.
	seen := make(map[string]bool)
	for _, m := range mirrors {
		if seen[m.Name] {
			t.Errorf("duplicate mirror %q in results", m.Name)
		}
		seen[m.Name] = true
	}

	if len(mirrors) == 0 {
		t.Fatal("All() returned no mirrors")
	}
}

func TestTunasyncManager_ConcurrentAccess(t *testing.T) {
	mgr, cleanup := newTestManager(t)
	defer cleanup()

	// Ensure the cache is initially populated.
	pollMirrors(t, mgr, 5*time.Second)

	const (
		workers  = 20
		duration = 2 * time.Second
	)

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Collect errors from goroutines.
	var (
		errMu sync.Mutex
		errs  []string
	)

	recordErr := func(msg string) {
		errMu.Lock()
		errs = append(errs, msg)
		errMu.Unlock()
	}

	for i := 0; i < workers; i++ {
		// All() reader: verify no duplicate names appear.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				seen := make(map[string]bool)
				for m := range mgr.All() {
					if seen[m.Name] {
						recordErr("All() returned duplicate mirror: " + m.Name)
					}
					seen[m.Name] = true
				}
			}
		}()

		// Get() reader: verify known mirror returns correct data and
		// unknown mirrors are not found.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if m, ok := mgr.Get("alpine"); ok && m.Name != "alpine" {
					recordErr("Get(alpine) returned mirror with wrong name: " + m.Name)
				}
				if _, ok := mgr.Get("nonexistent-mirror"); ok {
					recordErr("Get(nonexistent-mirror) unexpectedly returned ok=true")
				}
			}
		}()
	}

	wg.Wait()

	for _, e := range errs {
		t.Error(e)
	}
}
