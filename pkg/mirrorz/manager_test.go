// LLM usage: generated with deepseek-v4-pro and modified manually
package mirrorz

import (
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/openana/prism/pkg/mirrors"
	"github.com/rs/zerolog"
)

// --- mocks ---

// mockMirrorGetter implements mirrors.Getter for testing.
type mockMirrorGetter struct {
	mirrors  []mirrors.Mirror
	cacheTTL time.Duration
	age      time.Duration
}

func (m *mockMirrorGetter) All() (iter.Seq[mirrors.Mirror], time.Duration) {
	return slices.Values(m.mirrors), m.age
}

func (m *mockMirrorGetter) CacheTTL() time.Duration {
	return m.cacheTTL
}

// mockHelpProvider implements HelpURLProvider.
type mockHelpProvider struct {
	urls map[string]string
}

func (m *mockHelpProvider) HelpURL(name string) string {
	return m.urls[name]
}

// mockConfig implements Config.
type mockConfig struct {
	site Site
	info []Info
}

func (m mockConfig) Site() Site   { return m.site }
func (m mockConfig) Info() []Info { return m.info }

// --- tests ---

func TestManager_Mirrorz_ReturnsMirrorzWithSync(t *testing.T) {
	mirrorGetter := &mockMirrorGetter{
		mirrors: []mirrors.Mirror{
			{
				Name: "alpine",
				Sync: &mirrors.Sync{
					Status:       mirrors.Success,
					LastEnded:    1778201981,
					NextSchedule: 1780703762,
					Upstream:     "rsync://example.com/alpine",
					Size:         4 * 1024 * 1024 * 1024 * 1024,
				},
				Metadata: &mirrors.Metadata{
					Desc: "Alpine Linux",
					URL:  "/alpine",
					Type: mirrors.Rsync,
				},
			},
		},
		cacheTTL: 500 * time.Second,
	}

	helpProvider := &mockHelpProvider{
		urls: map[string]string{
			"alpine": "https://example.org/help/alpine",
		},
	}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
		info: []Info{{Distro: "Alpine", Category: "os"}},
	}

	mgr := NewManager(cfg, mirrorGetter, helpProvider, zerolog.Nop())

	mz, age, err := mgr.Mirrorz()
	_ = age
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if mz.Site.URL != "https://example.org" {
		t.Errorf("Site.URL = %q", mz.Site.URL)
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
	if m.URL != "/alpine" {
		t.Errorf("URL = %q", m.URL)
	}
	if m.Help != "https://example.org/help/alpine" {
		t.Errorf("Help = %q, want %q", m.Help, "https://example.org/help/alpine")
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
	mirrorGetter := &mockMirrorGetter{
		mirrors: []mirrors.Mirror{
			{
				Name:     "gentoo",
				Metadata: &mirrors.Metadata{Desc: "Gentoo Linux", Type: mirrors.Rsync},
			},
		},
		cacheTTL: 500 * time.Second,
	}

	helpProvider := &mockHelpProvider{urls: nil}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
	}

	mgr := NewManager(cfg, mirrorGetter, helpProvider, zerolog.Nop())

	mz, age, err := mgr.Mirrorz()
	_ = age
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if len(mz.Mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1 (gentoo)", len(mz.Mirrors))
	}

	m := mz.Mirrors[0]
	if m.Cname != "gentoo" {
		t.Errorf("Cname = %q, want %q", m.Cname, "gentoo")
	}
	if !m.Disable {
		t.Error("Disable should be true when Sync is nil")
	}
	if m.Status != "U" {
		t.Errorf("Status = %q, want %q", m.Status, "U")
	}
}

func TestManager_Mirrorz_EmptyMirrors(t *testing.T) {
	mirrorGetter := &mockMirrorGetter{
		mirrors:  []mirrors.Mirror{},
		cacheTTL: 500 * time.Second,
	}

	helpProvider := &mockHelpProvider{urls: nil}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
	}

	mgr := NewManager(cfg, mirrorGetter, helpProvider, zerolog.Nop())

	mz, age, err := mgr.Mirrorz()
	_ = age
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if len(mz.Mirrors) != 0 {
		t.Errorf("got %d mirrors, want 0", len(mz.Mirrors))
	}
	if mz.Site.URL != "https://example.org" {
		t.Errorf("Site.URL = %q", mz.Site.URL)
	}
}

func TestManager_Mirrorz_SkipsNonMirrorTypes(t *testing.T) {
	mirrorGetter := &mockMirrorGetter{
		mirrors: []mirrors.Mirror{
			{
				Name: "alpine",
				Metadata: &mirrors.Metadata{
					Desc: "Alpine Linux",
					URL:  "/alpine",
					Type: mirrors.Rsync,
				},
				Sync: &mirrors.Sync{Status: mirrors.Success},
			},
			{
				Name: "redirect-mirror",
				Metadata: &mirrors.Metadata{
					Desc: "Some Redirect",
					URL:  "/redirect",
					Type: mirrors.Redirect,
				},
				Sync: &mirrors.Sync{Status: mirrors.Success},
			},
		},
		cacheTTL: 500 * time.Second,
	}

	helpProvider := &mockHelpProvider{urls: nil}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
	}

	mgr := NewManager(cfg, mirrorGetter, helpProvider, zerolog.Nop())

	mz, _, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	// Redirect type should be skipped; only Rsync appears.
	if len(mz.Mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1 (redirect type skipped)", len(mz.Mirrors))
	}
	if mz.Mirrors[0].Cname != "alpine" {
		t.Errorf("Cname = %q, want %q", mz.Mirrors[0].Cname, "alpine")
	}
}

func TestManager_Mirrorz_HelpNotSetWhenProviderReturnsEmpty(t *testing.T) {
	mirrorGetter := &mockMirrorGetter{
		mirrors: []mirrors.Mirror{
			{
				Name: "alpine",
				Metadata: &mirrors.Metadata{
					Desc: "Alpine Linux",
					Type: mirrors.Rsync,
				},
				Sync: &mirrors.Sync{Status: mirrors.Success},
			},
		},
		cacheTTL: 500 * time.Second,
	}

	// Help provider returns empty string for this mirror.
	helpProvider := &mockHelpProvider{urls: map[string]string{}}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
	}

	mgr := NewManager(cfg, mirrorGetter, helpProvider, zerolog.Nop())

	mz, _, err := mgr.Mirrorz()
	if err != nil {
		t.Fatalf("Mirrorz() unexpected error: %v", err)
	}

	if len(mz.Mirrors) != 1 {
		t.Fatalf("got %d mirrors, want 1", len(mz.Mirrors))
	}

	if mz.Mirrors[0].Help != "" {
		t.Errorf("Help = %q, want empty (provider returned empty)", mz.Mirrors[0].Help)
	}
}

func TestManager_CacheTTL(t *testing.T) {
	mirrorGetter := &mockMirrorGetter{
		cacheTTL: 42 * time.Second,
	}

	cfg := mockConfig{
		site: Site{URL: "https://example.org", Abbr: "EX"},
	}

	mgr := NewManager(cfg, mirrorGetter, nil, zerolog.Nop())

	if got := mgr.CacheTTL(); got != 42*time.Second {
		t.Errorf("CacheTTL() = %v, want %v", got, 42*time.Second)
	}
}
