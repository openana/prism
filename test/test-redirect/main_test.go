// LLM usage: generated with deepseek-v4-pro and modified manually.
// Integration tests for redirect + Host header sniffing (v4/v6 FQDN selection).
package testredirect

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/server"
	"github.com/rs/zerolog"
)

// testServer wraps a running prism server for integration testing.
type testServer struct {
	srv     *server.Server
	cleanup func()
	port    int
	baseURL string
}

// newTestServer builds a prism server on a random free port with the given config.
func newTestServer(t *testing.T, cfg *config.Config) *testServer {
	t.Helper()

	port := freePort(t)
	cfg.HTTP.Listen = fmt.Sprintf("127.0.0.1:%d", port)

	srv, cleanup, err := server.InitializeServer(cfg)
	if err != nil {
		t.Fatalf("InitializeServer: %v", err)
	}

	ctx := context.Background()
	if err := srv.Run(ctx); err != nil {
		cleanup()
		t.Fatalf("Run: %v", err)
	}

	// Wait for server to be ready.
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitReady(t, baseURL, 5*time.Second) {
		srv.Stop(ctx)
		cleanup()
		t.Fatal("server did not become ready")
	}

	return &testServer{
		srv:     srv,
		cleanup: cleanup,
		port:    port,
		baseURL: baseURL,
	}
}

func (ts *testServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts.srv.Stop(ctx)
	ts.cleanup()
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitReady(t *testing.T, baseURL string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// noRedirectClient returns an http.Client that does not follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// --- Config builders ---

func baseConfig() *config.Config {
	return &config.Config{
		Log: config.Log{
			Level:  "error",
			Output: "stdout",
		},
		AccessLog: config.Log{
			Level:  "error",
			Output: "stdout",
		},
		HTTP: config.HTTP{
			ProtoHeader: "X-Forwarded-Proto",
		},
		Index: config.Index{
			CacheTTL:      "1m",
			CacheMaxBytes: "1MB",
		},
		SyncStatus: config.SyncStatus{
			CacheTTL: "5s",
		},
		Site: config.Site{
			URL:   "mirrors.example.com",
			URLv4: "mirrors4.example.com",
			URLv6: "mirrors6.example.com",
		},
	}
}

// cfgWithAllV4V6 returns a config where hosts and static mirrors have both v4 and v6 FQDNs.
func cfgWithAllV4V6() *config.Config {
	cfg := baseConfig()
	cfg.Hosts = []config.Host{
		{
			Name:   "h1",
			FQDN:   "mirrors.example.com",
			FQDNv4: "mirrors4.example.com",
			FQDNv6: "mirrors6.example.com",
			Index: config.HostIndex{
				Driver: "nginx",
				Nginx: config.IndexNginx{
					BaseURL: "http://127.0.0.1:1/",
				},
			},
			SyncStatus: config.HostSyncStatus{
				Driver: "tunasync",
				Tunasync: config.SyncStatusTunasync{
					Endpoint: "http://127.0.0.1:1",
				},
			},
			Mirrors: []config.Mirror{
				{Name: "alpine", URLPrefix: "/alpine/", RealURLPrefix: "/alpine/"},
				{Name: "ubuntu", URLPrefix: "/ubuntu/", RealURLPrefix: "/ubuntu/"},
			},
		},
	}
	cfg.StaticMirrors = []config.StaticMirror{
		{
			FQDN: "static.example.com", FQDNv4: "static4.example.com", FQDNv6: "static6.example.com",
			Name: "sm1", URLPrefix: "/sm/", RealURLPrefix: "/sm/",
		},
	}
	return cfg
}

// cfgWithoutV4V6 returns a config where hosts have no fqdn_v4/fqdn_v6 (use fqdn fallback).
func cfgWithoutV4V6() *config.Config {
	cfg := baseConfig()
	cfg.Hosts = []config.Host{
		{
			Name: "h1",
			FQDN: "mirrors.example.com",
			Index: config.HostIndex{
				Driver: "nginx",
				Nginx: config.IndexNginx{
					BaseURL: "http://127.0.0.1:1/",
				},
			},
			SyncStatus: config.HostSyncStatus{
				Driver: "tunasync",
				Tunasync: config.SyncStatusTunasync{
					Endpoint: "http://127.0.0.1:1",
				},
			},
			Mirrors: []config.Mirror{
				{Name: "alpine", URLPrefix: "/alpine/", RealURLPrefix: "/alpine/"},
			},
		},
	}
	cfg.StaticMirrors = []config.StaticMirror{
		{
			FQDN: "static.example.com",
			Name: "sm1", URLPrefix: "/sm/", RealURLPrefix: "/sm/",
		},
	}
	return cfg
}

// cfgWithoutSiteV4 returns a config where site.url_v4 is empty.
func cfgWithoutSiteV4() *config.Config {
	cfg := cfgWithAllV4V6()
	cfg.Site.URLv4 = ""
	return cfg
}

// --- Helper ---

func doRequest(t *testing.T, client *http.Client, method, url, host, proto string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if host != "" {
		req.Host = host
	}
	if proto != "" {
		req.Header.Set("X-Forwarded-Proto", proto)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("status = %d, want %d", resp.StatusCode, want)
	}
}

func assertLocation(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	loc := resp.Header.Get("Location")
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

// --- Tests ---

// TestRedirect_SiteURL uses the default (IPv4) site.url host header.
func TestRedirect_SiteURL(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")
}

// TestRedirect_SiteURLv4 matches site.url_v4 → uses fqdn_v4.
func TestRedirect_SiteURLv4(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors4.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors4.example.com/alpine/v3.21/")
}

// TestRedirect_SiteURLv6 matches site.url_v6 → uses fqdn_v6.
func TestRedirect_SiteURLv6(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors6.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors6.example.com/alpine/v3.21/")
}

// TestRedirect_StaticMirrorV4 matches site.url_v4 on a static mirror.
func TestRedirect_StaticMirrorV4(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/sm/somefile", "mirrors4.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://static4.example.com/sm/somefile")
}

// TestRedirect_StaticMirrorV6 matches site.url_v6 on a static mirror.
func TestRedirect_StaticMirrorV6(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/sm/somefile", "mirrors6.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://static6.example.com/sm/somefile")
}

// TestRedirect_NoV4V6_FallbackToFQDN when fqdn_v4/fqdn_v6 are not set, matching
// site.url_v4 or site.url_v6 falls back to fqdn.
func TestRedirect_NoV4V6_FallbackToFQDN(t *testing.T) {
	ts := newTestServer(t, cfgWithoutV4V6())
	defer ts.Close()
	client := noRedirectClient()

	// Match v4 host → should fall back to fqdn
	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors4.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")

	// Match v6 host → should fall back to fqdn
	resp = doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors6.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")

	// Static mirror v4 host → fallback to fqdn
	resp = doRequest(t, client, "GET", ts.baseURL+"/sm/somefile", "mirrors4.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://static.example.com/sm/somefile")
}

// TestRedirect_UnknownHost falls back to fqdn.
func TestRedirect_UnknownHost(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "random.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")
}

// TestRedirect_NoHostHeader falls back to fqdn.
func TestRedirect_NoHostHeader(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")
}

// TestRedirect_PathNotFound returns 404 for paths not in the trie.
func TestRedirect_PathNotFound(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/nonexistent/foo", "mirrors.example.com", "")
	assertStatus(t, resp, http.StatusNotFound)
}

// TestRedirect_SiteV4NotConfigured when site.url_v4 is empty, only site.url matching works.
func TestRedirect_SiteV4NotConfigured(t *testing.T) {
	ts := newTestServer(t, cfgWithoutSiteV4())
	defer ts.Close()
	client := noRedirectClient()

	// site.url still works
	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")

	// v4 host falls back to fqdn (site.url_v4 is empty, can't match)
	resp = doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors4.example.com", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")
}

// --- Proto sniffing tests ---

// TestRedirect_ProtoHTTPS sniffs X-Forwarded-Proto: https → redirect uses https scheme.
func TestRedirect_ProtoHTTPS(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors.example.com", "https")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "https://mirrors.example.com/alpine/v3.21/")
}

// TestRedirect_ProtoHTTP sniffs X-Forwarded-Proto: http → redirect uses http scheme.
func TestRedirect_ProtoHTTP(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors.example.com", "http")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors.example.com/alpine/v3.21/")
}

// TestRedirect_ProtoHTTPS_V4 combines proto sniffing with v4 FQDN selection.
func TestRedirect_ProtoHTTPS_V4(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors4.example.com", "https")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "https://mirrors4.example.com/alpine/v3.21/")
}

// TestRedirect_ProtoHTTPS_V6 combines proto sniffing with v6 FQDN selection.
func TestRedirect_ProtoHTTPS_V6(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors6.example.com", "https")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "https://mirrors6.example.com/alpine/v3.21/")
}

// TestRedirect_ProtoHTTPS_UnknownHost combines proto sniffing with unknown host fallback.
func TestRedirect_ProtoHTTPS_UnknownHost(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "random.example.com", "https")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "https://mirrors.example.com/alpine/v3.21/")
}

// --- Port stripping tests ---

// TestRedirect_HostWithPort strips the port from the Host header before matching.
func TestRedirect_HostWithPort(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors4.example.com:8080", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors4.example.com/alpine/v3.21/")
}

// TestRedirect_HostWithPort_V6 strips port from v6 host.
func TestRedirect_HostWithPort_V6(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "mirrors6.example.com:8443", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors6.example.com/alpine/v3.21/")
}

// --- Case insensitivity tests ---

// TestRedirect_HostCaseInsensitive matches Host header case-insensitively.
func TestRedirect_HostCaseInsensitive(t *testing.T) {
	ts := newTestServer(t, cfgWithAllV4V6())
	defer ts.Close()
	client := noRedirectClient()

	resp := doRequest(t, client, "GET", ts.baseURL+"/alpine/v3.21/", "Mirrors4.Example.COM", "")
	assertStatus(t, resp, http.StatusMovedPermanently)
	assertLocation(t, resp, "http://mirrors4.example.com/alpine/v3.21/")
}

func TestMain(m *testing.M) {
	// Suppress zerolog output during tests.
	zerolog.SetGlobalLevel(zerolog.Disabled)
	m.Run()
}
