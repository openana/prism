package index

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// mockNginxFetcherConfig implements NginxFetcherConfig.
type mockNginxFetcherConfig struct {
	baseURL    string
	timeout    time.Duration
	timeLayout string
}

func (m mockNginxFetcherConfig) BaseURL() string        { return m.baseURL }
func (m mockNginxFetcherConfig) Timeout() time.Duration { return m.timeout }
func (m mockNginxFetcherConfig) TimeLayout() string     { return m.timeLayout }

func TestNginxFetcher_AllOrErr_ParsesAlpineJSON(t *testing.T) {
	// Read testdata.
	data, err := os.ReadFile(filepath.Join("testdata", "alpine.json"))
	if err != nil {
		t.Fatalf("failed to read alpine.json: %v", err)
	}

	// Spin up a test server serving alpine.json.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	it, err := fetcher.AllOrErr(context.Background(), []byte(""))
	if err != nil {
		t.Fatalf("AllOrErr() unexpected error: %v", err)
	}

	var entries []Entry
	for e := range it {
		entries = append(entries, e)
	}

	if len(entries) != 13 {
		t.Fatalf("got %d entries, want 13", len(entries))
	}

	// Check first entry: "edge" directory.
	if entries[0].Name != "edge" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "edge")
	}
	if entries[0].Type != Directory {
		t.Errorf("entries[0].Type = %v, want Directory", entries[0].Type)
	}

	// Check a file entry: "MIRRORS.txt" (index 11).
	mirrors := entries[11]
	if mirrors.Name != "MIRRORS.txt" {
		t.Errorf("MIRRORS.txt Name = %q", mirrors.Name)
	}
	if mirrors.Type != File {
		t.Errorf("MIRRORS.txt Type = %v, want File", mirrors.Type)
	}
	if mirrors.Size != 3632 {
		t.Errorf("MIRRORS.txt Size = %d, want 3632", mirrors.Size)
	}

	// Check last entry: "last-updated" file.
	last := entries[12]
	if last.Name != "last-updated" {
		t.Errorf("last-updated Name = %q", last.Name)
	}
	if last.Size != 11 {
		t.Errorf("last-updated Size = %d, want 11", last.Size)
	}
}

func TestNginxFetcher_AllOrErr_StripsLeadingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	// Path with leading slash should be trimmed to avoid double slash.
	_, err := fetcher.AllOrErr(context.Background(), []byte("/some/path"))
	if err != nil {
		t.Fatalf("AllOrErr() unexpected error: %v", err)
	}

	// The resulting path should not have a leading double slash.
	if gotPath != "/some/path/" {
		t.Errorf("request path = %q, want %q", gotPath, "/some/path/")
	}
}

func TestNginxFetcher_AllOrErr_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	_, err := fetcher.AllOrErr(context.Background(), []byte(""))
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestNginxFetcher_AllOrErr_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	_, err := fetcher.AllOrErr(context.Background(), []byte(""))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestNginxFetcher_AllOrErr_SkipsUnparseableMtime(t *testing.T) {
	// One entry with bad mtime, one good entry.
	resp := `[
		{"name":"bad","type":"file","mtime":"not-a-date","size":1},
		{"name":"good","type":"file","mtime":"Wed, 30 Sep 2015 07:58:27 GMT","size":2}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	it, err := fetcher.AllOrErr(context.Background(), []byte(""))
	if err != nil {
		t.Fatalf("AllOrErr() unexpected error: %v", err)
	}

	var entries []Entry
	for e := range it {
		entries = append(entries, e)
	}

	// Only the good entry should be yielded; bad entry skipped with a log warning.
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (bad mtime entry should be skipped)", len(entries))
	}
	if entries[0].Name != "good" {
		t.Errorf("got entry name %q, want %q", entries[0].Name, "good")
	}
}

func TestNginxFetcher_AllOrErr_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxFetcherConfig{
		baseURL:    srv.URL + "/",
		timeout:    5 * time.Second,
		timeLayout: time.RFC1123,
	}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := fetcher.AllOrErr(ctx, []byte(""))
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
