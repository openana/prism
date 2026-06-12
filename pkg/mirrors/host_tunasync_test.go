// LLM usage: generated with deepseek-v4-pro and modified manually.
package mirrors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/go-units"
	"github.com/rs/zerolog"
)

// mockTunasyncHostConfig implements TunasyncHostConfig.
type mockTunasyncHostConfig struct {
	name     string
	endpoint string
}

func (m mockTunasyncHostConfig) Name() string     { return m.name }
func (m mockTunasyncHostConfig) Endpoint() string { return m.endpoint }
func (m mockTunasyncHostConfig) IsHostConfig()    {}

func TestTunasyncHost_FetchMirrors_ParsesTunasyncJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tunasync.json"))
	if err != nil {
		t.Fatalf("failed to read tunasync.json: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	host := NewTunasyncHost(mockTunasyncHostConfig{
		name:     "test-host",
		endpoint: srv.URL,
	}, zerolog.Nop())

	mirrors, err := host.FetchMirrors(context.Background())
	if err != nil {
		t.Fatalf("FetchMirrors() unexpected error: %v", err)
	}

	if len(mirrors) != 60 {
		t.Fatalf("got %d mirrors, want 60", len(mirrors))
	}

	// alpine: failed status, 4.00T size.
	alpine := mirrors[0]
	if alpine.Name != "alpine" {
		t.Errorf("alpine.Name = %q, want %q", alpine.Name, "alpine")
	}
	if alpine.SyncStatus == nil {
		t.Fatal("alpine.SyncStatus is nil")
	}
	if alpine.SyncStatus.Status != "failed" {
		t.Errorf("alpine.Status = %q, want %q", alpine.SyncStatus.Status, "failed")
	}
	expectedSize, _ := units.FromHumanSize("4.00T")
	if alpine.SyncStatus.Size != expectedSize {
		t.Errorf("alpine.Size = %d, want %d", alpine.SyncStatus.Size, expectedSize)
	}
	if alpine.SyncStatus.Upstream != "rsync://mirrors.tuna.tsinghua.edu.cn/alpine/" {
		t.Errorf("alpine.Upstream = %q", alpine.SyncStatus.Upstream)
	}

	// centos-vault: success status.
	findMirror := func(name string) *Mirror {
		for i := range mirrors {
			if mirrors[i].Name == name {
				return &mirrors[i]
			}
		}
		return nil
	}

	cv := findMirror("centos-vault")
	if cv == nil {
		t.Fatal("centos-vault not found")
	}
	if cv.SyncStatus.Status != "success" {
		t.Errorf("centos-vault.Status = %q, want %q", cv.SyncStatus.Status, "success")
	}

	// debian-cd: syncing status.
	dc := findMirror("debian-cd")
	if dc == nil {
		t.Fatal("debian-cd not found")
	}
	if dc.SyncStatus.Status != "syncing" {
		t.Errorf("debian-cd.Status = %q, want %q", dc.SyncStatus.Status, "syncing")
	}

	// homebrew-bundle.git: paused status, unknown size.
	hbb := findMirror("homebrew-bundle.git")
	if hbb == nil {
		t.Fatal("homebrew-bundle.git not found")
	}
	if hbb.SyncStatus.Status != "paused" {
		t.Errorf("homebrew-bundle.git.Status = %q, want %q", hbb.SyncStatus.Status, "paused")
	}
	if hbb.SyncStatus.Size != -1 {
		t.Errorf("homebrew-bundle.git.Size = %d, want -1 (unknown)", hbb.SyncStatus.Size)
	}

	// Verify SyncStatus fields are populated on a sample mirror.
	archlinux := findMirror("archlinux")
	if archlinux == nil {
		t.Fatal("archlinux not found")
	}
	if archlinux.SyncStatus.LastUpdate == 0 {
		t.Error("archlinux.LastUpdate is zero")
	}
	if archlinux.SyncStatus.LastStarted == 0 {
		t.Error("archlinux.LastStarted is zero")
	}
	if archlinux.SyncStatus.LastEnded == 0 {
		t.Error("archlinux.LastEnded is zero")
	}
	if archlinux.SyncStatus.NextSchedule == 0 {
		t.Error("archlinux.NextSchedule is zero")
	}
}

func TestTunasyncHost_FetchMirrors_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	host := NewTunasyncHost(mockTunasyncHostConfig{
		name:     "test-host",
		endpoint: srv.URL,
	}, zerolog.Nop())

	_, err := host.FetchMirrors(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestTunasyncHost_FetchMirrors_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json {{{"))
	}))
	defer srv.Close()

	host := NewTunasyncHost(mockTunasyncHostConfig{
		name:     "test-host",
		endpoint: srv.URL,
	}, zerolog.Nop())

	_, err := host.FetchMirrors(context.Background())
	if err == nil {
		t.Fatal("expected decode error for invalid JSON, got nil")
	}
}

func TestTunasyncHost_FetchMirrors_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	host := NewTunasyncHost(mockTunasyncHostConfig{
		name:     "test-host",
		endpoint: srv.URL,
	}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := host.FetchMirrors(ctx)
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
